package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dublyo/dockerizer/internal/ai"
	"github.com/dublyo/dockerizer/internal/detector"
	"github.com/dublyo/dockerizer/internal/generator"
	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers/golang"
	"github.com/dublyo/dockerizer/providers/java"
	"github.com/dublyo/dockerizer/providers/nodejs"
	"github.com/dublyo/dockerizer/providers/php"
	"github.com/dublyo/dockerizer/providers/python"
	"github.com/dublyo/dockerizer/providers/ruby"
	"github.com/dublyo/dockerizer/providers/rust"
)

// DockerizeResult is the JSON output structure
type DockerizeResult struct {
	Success    bool     `json:"success"`
	Language   string   `json:"language,omitempty"`
	Framework  string   `json:"framework,omitempty"`
	Version    string   `json:"version,omitempty"`
	Confidence int      `json:"confidence,omitempty"`
	Files      []string `json:"files,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// executeDockerize runs the full dockerizer workflow
func executeDockerize(path, outputDir string, forceAI, overwrite, includeCompose, includeIgnore, includeEnv bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Step 1: Scan the repository
	printInfo("Scanning %s...", path)
	scan, err := scanner.New().Scan(ctx, path)
	if err != nil {
		return outputError("scan failed", err)
	}
	printVerbose("Found %d files in %d directories", len(scan.FileTree.Files), len(scan.FileTree.Dirs))

	// Step 2: Detect the stack
	printInfo("Detecting stack...")
	registry := setupRegistry()
	det := detector.New(registry)
	result, err := det.Detect(ctx, scan)
	if err != nil {
		return outputError("detection failed", err)
	}

	// Configure generator options
	genOpts := []generator.Option{
		generator.WithOverwrite(overwrite),
		generator.WithCompose(includeCompose),
		generator.WithIgnore(includeIgnore),
		generator.WithEnv(includeEnv),
	}

	// Setup AI provider for fallback if needed
	var aiProvider ai.Provider
	useAI := !result.Detected || result.Confidence < 80 || forceAI

	if useAI {
		aiProvider = getAIProvider()
		if aiProvider != nil {
			genOpts = append(genOpts, generator.WithAIProvider(aiProvider))
		}
	}

	// If no stack detected and no AI available, fail
	if !result.Detected {
		if aiProvider == nil {
			printInfo("")
			printInfo("No stack detected. To use AI-powered detection:")
			printInfo("  1. Set ANTHROPIC_API_KEY, OPENAI_API_KEY, or run Ollama locally")
			printInfo("  2. Run with --ai flag: dockerizer --ai %s", path)
			return outputError("no stack detected", fmt.Errorf("could not identify the project type; try using --ai with an API key"))
		}
		printInfo("No stack detected, using AI generation...")
	} else {
		printInfo("Detected: %s/%s (confidence: %d%%)", result.Language, result.Framework, result.Confidence)
		if useAI && aiProvider != nil {
			printInfo("AI fallback enabled (confidence: %d%%)", result.Confidence)
		}
	}

	// Step 3: Generate files
	printInfo("Generating Docker configuration...")

	gen := generator.New(genOpts...)

	// Use AI generation if stack not detected or confidence is low
	var output *generator.Output
	if useAI && aiProvider != nil {
		output, err = gen.GenerateWithAIFallback(ctx, result, scan, outputDir)
	} else {
		output, err = gen.Generate(result, outputDir)
	}

	if err != nil {
		return outputError("generation failed", err)
	}

	// Output results
	if jsonOut {
		var files []string
		for f := range output.Files {
			files = append(files, f)
		}
		return outputJSON(DockerizeResult{
			Success:    true,
			Language:   result.Language,
			Framework:  result.Framework,
			Version:    result.Version,
			Confidence: result.Confidence,
			Files:      files,
		})
	}

	// Print generated files
	printSuccess("Generated files:")
	for filename := range output.Files {
		printInfo("  - %s", filename)
	}

	// Print next steps
	printInfo("")
	printInfo("Next steps:")
	printInfo("  1. Review the generated Dockerfile")
	printInfo("  2. Update .env.example with your values")
	printInfo("  3. Build: docker compose build")
	printInfo("  4. Run: docker compose up")

	return nil
}

// setupRegistry creates and configures the provider registry
func setupRegistry() *detector.Registry {
	registry := detector.NewRegistry()

	// Register all providers
	nodejs.RegisterAll(registry)
	python.RegisterAll(registry)
	golang.RegisterAll(registry)
	rust.RegisterAll(registry)
	ruby.RegisterAll(registry)
	php.RegisterAll(registry)
	java.RegisterAll(registry)

	return registry
}

// outputError handles error output
func outputError(context string, err error) error {
	if jsonOut {
		_ = outputJSON(DockerizeResult{
			Success: false,
			Error:   fmt.Sprintf("%s: %v", context, err),
		})
	} else {
		printError("%s: %v", context, err)
	}
	return err
}

// outputJSON prints JSON output
func outputJSON(result DockerizeResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// getAIProvider creates an AI provider from environment variables
func getAIProvider() ai.Provider {
	// Try Anthropic first
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = "claude-3-5-haiku-20241022"
		}
		provider := ai.NewAnthropicProvider(apiKey, model)
		if provider.IsAvailable() {
			printVerbose("Using Anthropic AI provider (model: %s)", model)
			return provider
		}
	}

	// Try OpenAI
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = "gpt-4o-mini"
		}
		provider := ai.NewOpenAIProvider(apiKey, model)
		if provider.IsAvailable() {
			printVerbose("Using OpenAI AI provider (model: %s)", model)
			return provider
		}
	}

	// Try Ollama (local)
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "llama3"
	}
	provider := ai.NewOllamaProvider(baseURL, model)
	if provider.IsAvailable() {
		printVerbose("Using Ollama AI provider (model: %s)", model)
		return provider
	}

	return nil
}
