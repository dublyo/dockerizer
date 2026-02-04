package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dublyo/dockerizer/internal/ai"
	"github.com/dublyo/dockerizer/internal/config"
	"github.com/dublyo/dockerizer/internal/detector"
	"github.com/dublyo/dockerizer/internal/generator"
	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Interactive Docker configuration setup",
	Long: `Interactively set up Docker configuration for your project.

This command guides you through:
  1. Stack detection and confirmation
  2. AI provider selection (optional)
  3. Configuration customization
  4. File generation

Examples:
  dockerizer init
  dockerizer init ./my-project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Make path absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	// Welcome message
	fmt.Println()
	fmt.Println("  Dockerizer - Interactive Setup")
	fmt.Println("  https://dockerizer.dev")
	fmt.Println()

	// Step 1: Scan and detect
	fmt.Printf("  Scanning %s...\n", absPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	scan, err := scanner.New(scanner.WithIgnoreHidden(false)).Scan(ctx, absPath)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	registry := setupRegistry()
	det := detector.New(registry)
	result, err := det.Detect(ctx, scan)
	if err != nil {
		return fmt.Errorf("detection failed: %w", err)
	}

	// Step 2: Show detection results
	fmt.Println()
	if result.Detected {
		fmt.Printf("  Detected Stack:\n")
		fmt.Printf("    Language:   %s\n", result.Language)
		fmt.Printf("    Framework:  %s\n", result.Framework)
		if result.Version != "" {
			fmt.Printf("    Version:    %s\n", result.Version)
		}
		fmt.Printf("    Confidence: %d%%\n", result.Confidence)
		fmt.Println()

		// Ask for confirmation
		if result.Confidence < 90 {
			fmt.Print("  Detection confidence is low. Use AI to improve? [Y/n]: ")
		} else {
			fmt.Print("  Proceed with this detection? [Y/n]: ")
		}
		response := readLine(reader)
		if strings.ToLower(response) == "n" {
			result.Detected = false // Force AI mode
		}
	}

	// Step 3: AI Configuration (if needed or requested)
	var aiProvider ai.Provider
	if !result.Detected || result.Confidence < 80 {
		fmt.Println()
		fmt.Println("  AI-Powered Generation")
		fmt.Println("  ---------------------")
		fmt.Println()
		fmt.Println("  Select AI provider:")
		fmt.Println("    1. Anthropic (Claude) - Recommended")
		fmt.Println("    2. OpenAI (GPT-4)")
		fmt.Println("    3. Ollama (Local)")
		fmt.Println("    4. Skip AI (use template only)")
		fmt.Println()
		fmt.Print("  Choice [1-4]: ")

		choice := readLine(reader)
		switch choice {
		case "1", "":
			aiProvider = configureAnthropic(reader)
		case "2":
			aiProvider = configureOpenAI(reader)
		case "3":
			aiProvider = configureOllama(reader)
		case "4":
			// Skip AI
			if !result.Detected {
				return fmt.Errorf("cannot proceed without AI - no stack detected")
			}
		}
	}

	// Step 4: Generation options
	fmt.Println()
	fmt.Println("  Generation Options")
	fmt.Println("  ------------------")
	fmt.Println()

	// Check existing files
	existingFiles := checkExistingFiles(absPath)
	overwrite := false
	if len(existingFiles) > 0 {
		fmt.Println("  Existing files found:")
		for _, f := range existingFiles {
			fmt.Printf("    - %s\n", f)
		}
		fmt.Println()
		fmt.Print("  Overwrite existing files? [y/N]: ")
		if strings.ToLower(readLine(reader)) == "y" {
			overwrite = true
		}
	}

	// Ask about compose/ignore/env
	fmt.Print("  Generate docker-compose.yml? [Y/n]: ")
	includeCompose := strings.ToLower(readLine(reader)) != "n"

	fmt.Print("  Generate .dockerignore? [Y/n]: ")
	includeIgnore := strings.ToLower(readLine(reader)) != "n"

	fmt.Print("  Generate .env.example? [Y/n]: ")
	includeEnv := strings.ToLower(readLine(reader)) != "n"

	// Step 5: Generate
	fmt.Println()
	fmt.Println("  Generating Docker configuration...")

	genOpts := []generator.Option{
		generator.WithOverwrite(overwrite),
		generator.WithCompose(includeCompose),
		generator.WithIgnore(includeIgnore),
		generator.WithEnv(includeEnv),
	}

	if aiProvider != nil {
		genOpts = append(genOpts, generator.WithAIProvider(aiProvider))
	}

	gen := generator.New(genOpts...)

	var output *generator.Output
	if aiProvider != nil {
		output, err = gen.GenerateWithAIFallback(ctx, result, scan, absPath)
	} else {
		output, err = gen.Generate(result, absPath)
	}

	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	// Step 6: Summary
	fmt.Println()
	fmt.Println("  Generated files:")
	for filename := range output.Files {
		fmt.Printf("    - %s\n", filename)
	}

	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    1. Review the generated Dockerfile")
	fmt.Println("    2. Update .env.example with your values")
	fmt.Println("    3. Build: docker compose build")
	fmt.Println("    4. Run:   docker compose up")
	fmt.Println()

	// Ask to save config
	fmt.Print("  Save AI configuration for future use? [y/N]: ")
	if strings.ToLower(readLine(reader)) == "y" {
		saveConfig(aiProvider)
	}

	return nil
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func configureAnthropic(reader *bufio.Reader) ai.Provider {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println()
		fmt.Println("  Anthropic API Key required.")
		fmt.Println("  Get one at: https://console.anthropic.com/")
		fmt.Println()
		fmt.Print("  API Key: ")
		apiKey = readLine(reader)
	}

	if apiKey == "" {
		return nil
	}

	fmt.Println()
	fmt.Println("  Select model:")
	fmt.Println("    1. claude-3-5-haiku-20241022 (Fast, recommended)")
	fmt.Println("    2. claude-3-5-sonnet-20241022 (Better quality)")
	fmt.Println()
	fmt.Print("  Choice [1-2]: ")

	model := "claude-3-5-haiku-20241022"
	if readLine(reader) == "2" {
		model = "claude-3-5-sonnet-20241022"
	}

	provider := ai.NewAnthropicProvider(apiKey, model)
	if !provider.IsAvailable() {
		fmt.Println("  Warning: Could not connect to Anthropic API")
		return nil
	}

	fmt.Printf("  Using Anthropic (%s)\n", model)
	return provider
}

func configureOpenAI(reader *bufio.Reader) ai.Provider {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println()
		fmt.Println("  OpenAI API Key required.")
		fmt.Println("  Get one at: https://platform.openai.com/api-keys")
		fmt.Println()
		fmt.Print("  API Key: ")
		apiKey = readLine(reader)
	}

	if apiKey == "" {
		return nil
	}

	fmt.Println()
	fmt.Println("  Select model:")
	fmt.Println("    1. gpt-4o-mini (Fast, cheap)")
	fmt.Println("    2. gpt-4o (Better quality)")
	fmt.Println()
	fmt.Print("  Choice [1-2]: ")

	model := "gpt-4o-mini"
	if readLine(reader) == "2" {
		model = "gpt-4o"
	}

	provider := ai.NewOpenAIProvider(apiKey, model)
	if !provider.IsAvailable() {
		fmt.Println("  Warning: Could not connect to OpenAI API")
		return nil
	}

	fmt.Printf("  Using OpenAI (%s)\n", model)
	return provider
}

func configureOllama(reader *bufio.Reader) ai.Provider {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	fmt.Println()
	fmt.Printf("  Ollama URL [%s]: ", baseURL)
	if url := readLine(reader); url != "" {
		baseURL = url
	}

	fmt.Println()
	fmt.Println("  Select model:")
	fmt.Println("    1. llama3 (Good balance)")
	fmt.Println("    2. codellama (Code-focused)")
	fmt.Println("    3. mistral (Fast)")
	fmt.Println("    4. Custom model")
	fmt.Println()
	fmt.Print("  Choice [1-4]: ")

	models := map[string]string{
		"1": "llama3",
		"2": "codellama",
		"3": "mistral",
	}

	choice := readLine(reader)
	model := models[choice]
	if choice == "4" || model == "" {
		fmt.Print("  Model name: ")
		model = readLine(reader)
	}
	if model == "" {
		model = "llama3"
	}

	provider := ai.NewOllamaProvider(baseURL, model)
	if !provider.IsAvailable() {
		fmt.Println("  Warning: Could not connect to Ollama")
		fmt.Println("  Make sure Ollama is running: ollama serve")
		return nil
	}

	fmt.Printf("  Using Ollama (%s)\n", model)
	return provider
}

func checkExistingFiles(path string) []string {
	files := []string{"Dockerfile", "docker-compose.yml", ".dockerignore", ".env.example"}
	var existing []string
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(path, f)); err == nil {
			existing = append(existing, f)
		}
	}
	return existing
}

func saveConfig(provider ai.Provider) {
	if provider == nil {
		return
	}

	cfg := config.DefaultConfig()

	// Try to determine provider type from the provider
	// This is a simplified version - in reality you'd want to store this info
	configPath := filepath.Join(os.Getenv("HOME"), ".dockerizer.yml")

	if err := cfg.Save(configPath); err != nil {
		fmt.Printf("  Warning: Could not save config: %v\n", err)
		return
	}

	fmt.Printf("  Config saved to %s\n", configPath)
}
