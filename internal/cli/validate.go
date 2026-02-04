package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ValidationOutput is the JSON output for validate command
type ValidationOutput struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationIssue `json:"errors,omitempty"`
	Warnings []ValidationIssue `json:"warnings,omitempty"`
}

// ValidationIssue represents a validation error or warning
type ValidationIssue struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

var validateCmd = &cobra.Command{
	Use:   "validate [dockerfile]",
	Short: "Validate a Dockerfile",
	Long: `Validate a Dockerfile for common issues and best practices.

This performs syntax validation and checks for common mistakes like:
- Missing FROM instruction
- Invalid instruction syntax
- Deprecated practices

Examples:
  dockerizer validate Dockerfile
  dockerizer validate ./my-project/Dockerfile`,
	Args: cobra.ExactArgs(1),
	RunE: runValidate,
}

func runValidate(cmd *cobra.Command, args []string) error {
	filepath := args[0]

	// Read the Dockerfile
	content, err := os.ReadFile(filepath)
	if err != nil {
		printError("failed to read file: %v", err)
		return err
	}

	// Validate
	errors, warnings := validateDockerfile(string(content))

	// Output
	if jsonOut {
		output := ValidationOutput{
			Valid:    len(errors) == 0,
			Errors:   errors,
			Warnings: warnings,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Text output
	if len(errors) == 0 && len(warnings) == 0 {
		printSuccess("Dockerfile is valid")
		return nil
	}

	if len(errors) > 0 {
		fmt.Println("Errors:")
		for _, e := range errors {
			fmt.Printf("  Line %d: %s\n", e.Line, e.Message)
		}
	}

	if len(warnings) > 0 {
		fmt.Println("Warnings:")
		for _, w := range warnings {
			fmt.Printf("  Line %d: %s\n", w.Line, w.Message)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed with %d errors", len(errors))
	}

	return nil
}

// validateDockerfile performs basic validation on a Dockerfile
func validateDockerfile(content string) ([]ValidationIssue, []ValidationIssue) {
	var errors []ValidationIssue
	var warnings []ValidationIssue

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	hasFROM := false
	hasMultipleFROM := false
	fromCount := 0

	validInstructions := map[string]bool{
		"FROM": true, "RUN": true, "CMD": true, "LABEL": true,
		"EXPOSE": true, "ENV": true, "ADD": true, "COPY": true,
		"ENTRYPOINT": true, "VOLUME": true, "USER": true,
		"WORKDIR": true, "ARG": true, "ONBUILD": true,
		"STOPSIGNAL": true, "HEALTHCHECK": true, "SHELL": true,
	}

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle line continuation
		for strings.HasSuffix(line, "\\") && scanner.Scan() {
			lineNum++
			line = strings.TrimSuffix(line, "\\") + " " + strings.TrimSpace(scanner.Text())
		}

		// Get the instruction
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		instruction := strings.ToUpper(parts[0])

		// Check for FROM instruction
		if instruction == "FROM" {
			hasFROM = true
			fromCount++
			if fromCount > 1 {
				hasMultipleFROM = true
			}
		}

		// Check for valid instruction
		if !validInstructions[instruction] && !strings.HasPrefix(instruction, "#") {
			// Could be a parser directive
			if lineNum == 1 && strings.Contains(line, "=") {
				continue // Likely a parser directive like "syntax="
			}
			errors = append(errors, ValidationIssue{
				Line:    lineNum,
				Message: fmt.Sprintf("unknown instruction: %s", instruction),
			})
		}

		// Check for deprecated MAINTAINER
		if instruction == "MAINTAINER" {
			warnings = append(warnings, ValidationIssue{
				Line:    lineNum,
				Message: "MAINTAINER is deprecated, use LABEL maintainer= instead",
			})
		}

		// Check for ADD with URL
		if instruction == "ADD" && len(parts) > 1 {
			if strings.HasPrefix(parts[1], "http://") || strings.HasPrefix(parts[1], "https://") {
				warnings = append(warnings, ValidationIssue{
					Line:    lineNum,
					Message: "consider using RUN curl/wget instead of ADD for URLs",
				})
			}
		}

		// Check for latest tag
		if instruction == "FROM" && len(parts) > 1 {
			image := parts[1]
			if strings.HasSuffix(image, ":latest") || (!strings.Contains(image, ":") && !strings.Contains(image, "@")) {
				warnings = append(warnings, ValidationIssue{
					Line:    lineNum,
					Message: "consider using a specific tag instead of 'latest'",
				})
			}
		}
	}

	// Check for required FROM
	if !hasFROM {
		errors = append(errors, ValidationIssue{
			Line:    1,
			Message: "Dockerfile must start with FROM instruction",
		})
	}

	// Info about multi-stage builds
	if hasMultipleFROM && verbose {
		printVerbose("Detected multi-stage build with %d stages", fromCount)
	}

	return errors, warnings
}
