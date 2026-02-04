package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dublyo/dockerizer/internal/detector"
	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/spf13/cobra"
)

// DetectionOutput is the JSON output for detect command
type DetectionOutput struct {
	Detected   bool                   `json:"detected"`
	Language   string                 `json:"language,omitempty"`
	Framework  string                 `json:"framework,omitempty"`
	Version    string                 `json:"version,omitempty"`
	Confidence int                    `json:"confidence,omitempty"`
	Provider   string                 `json:"provider,omitempty"`
	Candidates []CandidateOutput      `json:"candidates,omitempty"`
	Variables  map[string]interface{} `json:"variables,omitempty"`
}

// CandidateOutput represents a candidate in JSON output
type CandidateOutput struct {
	Provider   string `json:"provider"`
	Confidence int    `json:"confidence"`
}

var detectCmd = &cobra.Command{
	Use:   "detect [path]",
	Short: "Detect the stack without generating files",
	Long: `Detect the project's technology stack without generating any files.

This command scans the repository and attempts to identify the language,
framework, and version being used. It shows the confidence level and
any alternative candidates that were considered.

Examples:
  dockerizer detect .
  dockerizer detect ./my-project
  dockerizer detect --json ./my-project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDetect,
}

func init() {
	detectCmd.Flags().Bool("all", false, "Show all candidates, not just the best match")
}

func runDetect(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	showAll, _ := cmd.Flags().GetBool("all")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Scan
	printVerbose("Scanning %s...", path)
	scan, err := scanner.New().Scan(ctx, path)
	if err != nil {
		printError("scan failed: %v", err)
		return err
	}

	// Detect
	registry := setupRegistry()
	det := detector.New(registry)
	result, err := det.Detect(ctx, scan)
	if err != nil {
		printError("detection failed: %v", err)
		return err
	}

	// Output
	if jsonOut {
		return outputDetectJSON(result, showAll)
	}

	return outputDetectText(result, showAll)
}

func outputDetectJSON(result *detector.DetectionResult, showAll bool) error {
	output := DetectionOutput{
		Detected:   result.Detected,
		Language:   result.Language,
		Framework:  result.Framework,
		Version:    result.Version,
		Confidence: result.Confidence,
		Provider:   result.Provider,
		Variables:  result.Variables,
	}

	if showAll {
		for _, c := range result.Candidates {
			output.Candidates = append(output.Candidates, CandidateOutput{
				Provider:   c.Provider,
				Confidence: c.Confidence,
			})
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputDetectText(result *detector.DetectionResult, showAll bool) error {
	if !result.Detected {
		printInfo("No stack detected")
		printInfo("")
		printInfo("Possible reasons:")
		printInfo("  - Unrecognized project structure")
		printInfo("  - Missing package files (package.json, go.mod, etc.)")
		printInfo("  - Project type not yet supported")
		printInfo("")
		printInfo("Try running with --ai flag to use AI detection")
		return nil
	}

	// Print main result
	fmt.Println()
	fmt.Printf("  Language:   %s\n", result.Language)
	fmt.Printf("  Framework:  %s\n", result.Framework)
	if result.Version != "" {
		fmt.Printf("  Version:    %s\n", result.Version)
	}
	fmt.Printf("  Confidence: %d%%\n", result.Confidence)
	fmt.Printf("  Provider:   %s\n", result.Provider)
	fmt.Println()

	// Show variables if verbose
	if verbose && len(result.Variables) > 0 {
		fmt.Println("  Variables:")
		for k, v := range result.Variables {
			fmt.Printf("    %s: %v\n", k, v)
		}
		fmt.Println()
	}

	// Show candidates if requested
	if showAll && len(result.Candidates) > 1 {
		fmt.Println("  All candidates:")
		for i, c := range result.Candidates {
			marker := " "
			if i == 0 {
				marker = "→"
			}
			fmt.Printf("  %s %s (%d%%)\n", marker, c.Provider, c.Confidence)
		}
		fmt.Println()
	}

	// Confidence warning
	if result.Confidence < 80 {
		fmt.Println("  ⚠ Low confidence detection")
		fmt.Println("  Consider using --ai flag for better results")
		fmt.Println()
	}

	return nil
}
