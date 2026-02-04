// Package cli provides the command-line interface for dockerizer.
// Copyright (c) 2026 Dublyo. All rights reserved.
// Licensed under the MIT License.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version information (set at build time)
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"

	// Global flags
	verbose bool
	quiet   bool
	jsonOut bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dockerizer [path]",
	Short: "AI-powered Docker configuration generator",
	Long: `Dockerizer - AI-powered Docker configuration generator
https://dockerizer.dev by Dublyo

Automatically detect your project's stack and generate production-ready
Docker configurations including Dockerfile, docker-compose.yml,
.dockerignore, and .env.example.

Examples:
  # Dockerize the current directory
  dockerizer .

  # Dockerize a specific project
  dockerizer ./my-project

  # Only detect the stack without generating files
  dockerizer detect ./my-project

  # Generate with AI assistance
  dockerizer --ai ./my-project

For more information, visit: https://dockerizer.dev`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDockerize,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-essential output")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output in JSON format")

	// Dockerizer-specific flags
	rootCmd.Flags().Bool("ai", false, "Force AI generation even for detected stacks")
	rootCmd.Flags().Bool("no-compose", false, "Skip docker-compose.yml generation")
	rootCmd.Flags().Bool("no-ignore", false, "Skip .dockerignore generation")
	rootCmd.Flags().Bool("no-env", false, "Skip .env.example generation")
	rootCmd.Flags().BoolP("force", "f", false, "Overwrite existing files")
	rootCmd.Flags().StringP("output", "o", "", "Output directory (default: same as input)")

	// Add subcommands (agent, serve, recipe add themselves in their own init())
	rootCmd.AddCommand(detectCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(validateCmd)
}

// runDockerize is the main command handler
func runDockerize(cmd *cobra.Command, args []string) error {
	// Get path argument
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Get flags
	forceAI, _ := cmd.Flags().GetBool("ai")
	noCompose, _ := cmd.Flags().GetBool("no-compose")
	noIgnore, _ := cmd.Flags().GetBool("no-ignore")
	noEnv, _ := cmd.Flags().GetBool("no-env")
	force, _ := cmd.Flags().GetBool("force")
	outputDir, _ := cmd.Flags().GetString("output")

	if outputDir == "" {
		outputDir = path
	}

	// Run the dockerizer workflow
	return executeDockerize(path, outputDir, forceAI, force, !noCompose, !noIgnore, !noEnv)
}

// Print helpers
func printInfo(format string, args ...interface{}) {
	if !quiet {
		fmt.Printf(format+"\n", args...)
	}
}

func printVerbose(format string, args ...interface{}) {
	if verbose && !quiet {
		fmt.Printf(format+"\n", args...)
	}
}

func printError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

func printSuccess(format string, args ...interface{}) {
	if !quiet {
		fmt.Printf("✓ "+format+"\n", args...)
	}
}
