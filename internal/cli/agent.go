package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dublyo/dockerizer/internal/agent"
	"github.com/dublyo/dockerizer/internal/ai"
	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent [path]",
	Short: "Run in agent mode for iterative Docker configuration",
	Long: `Agent mode iteratively generates, builds, tests, and fixes Docker configurations.

It uses AI to analyze build errors and automatically fix issues until the
Docker image builds and runs successfully.

Examples:
  dockerizer agent ./my-project
  dockerizer agent --provider anthropic ./my-project
  dockerizer agent --max-attempts 10 ./my-project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAgent,
}

func init() {
	agentCmd.Flags().String("provider", "openai", "AI provider (openai, anthropic, ollama)")
	agentCmd.Flags().String("model", "", "Model to use (default depends on provider)")
	agentCmd.Flags().Int("max-attempts", 5, "Maximum fix attempts")
	agentCmd.Flags().String("instructions", "", "Additional instructions for the AI")

	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	providerName, _ := cmd.Flags().GetString("provider")
	model, _ := cmd.Flags().GetString("model")
	maxAttempts, _ := cmd.Flags().GetInt("max-attempts")
	instructions, _ := cmd.Flags().GetString("instructions")

	// Get API key from environment
	var apiKey string
	switch providerName {
	case "openai":
		apiKey = os.Getenv("OPENAI_API_KEY")
	case "anthropic":
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	case "ollama":
		// No API key needed for Ollama
	}

	if apiKey == "" && providerName != "ollama" {
		printError("API key not found. Set %s_API_KEY environment variable", providerName)
		return fmt.Errorf("missing API key")
	}

	// Create AI provider
	aiProvider, err := ai.NewProvider(ai.Config{
		Provider: providerName,
		APIKey:   apiKey,
		Model:    model,
	})
	if err != nil {
		return fmt.Errorf("failed to create AI provider: %w", err)
	}

	if !aiProvider.IsAvailable() {
		return fmt.Errorf("AI provider %s is not available", providerName)
	}

	// Scan the repository first
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	printInfo("Scanning %s...", path)
	scan, err := scanner.New().Scan(ctx, path)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Create and run agent
	ag := agent.New(agent.AgentConfig{
		AIProvider:  aiProvider,
		MaxAttempts: maxAttempts,
		WorkDir:     path,
		Verbose:     verbose,
	})

	// Monitor events in background
	go func() {
		for event := range ag.Events() {
			switch event.Type {
			case agent.EventStart:
				printInfo("Starting agent...")
			case agent.EventAnalyzing:
				printInfo("Analyzing: %s", event.Message)
			case agent.EventGenerating:
				printInfo("Generating Docker configuration...")
			case agent.EventBuilding:
				printInfo("Building Docker image...")
			case agent.EventTesting:
				printInfo("Testing container...")
			case agent.EventFixing:
				printInfo("Fixing issues: %s", event.Message)
			case agent.EventSuccess:
				printSuccess(event.Message)
			case agent.EventError:
				printError(event.Message)
			case agent.EventComplete:
				printInfo("Agent completed")
			}
		}
	}()

	// Run agent
	result, err := ag.Run(ctx, scan, instructions)
	if err != nil {
		return fmt.Errorf("agent failed: %w", err)
	}

	// Print results
	if result.Success {
		printSuccess("Docker configuration generated successfully after %d attempt(s)", len(result.Attempts))
		printInfo("")
		printInfo("Generated files:")
		printInfo("  - Dockerfile")
		printInfo("  - docker-compose.yml")
		printInfo("  - .dockerignore")
		printInfo("  - .env.example")
	} else {
		printError("Agent failed after %d attempts", len(result.Attempts))
		for i, attempt := range result.Attempts {
			if attempt.Error != "" {
				printError("  Attempt %d: %s", i+1, attempt.Error)
			}
		}
	}

	return nil
}
