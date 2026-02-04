package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dublyo/dockerizer/internal/detector"
	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// BuildPlan represents the intermediate representation of a Docker build
// Inspired by Nixpacks' plan concept
type BuildPlan struct {
	// Metadata
	Version   string `json:"version" yaml:"version"`
	Generator string `json:"generator" yaml:"generator"`

	// Detection results
	Detection DetectionPlan `json:"detection" yaml:"detection"`

	// Build phases (Nixpacks-inspired)
	Phases []BuildPhase `json:"phases" yaml:"phases"`

	// Variables available for templates
	Variables map[string]interface{} `json:"variables" yaml:"variables"`

	// Cache directories for Docker buildkit
	CacheDirs []CacheDir `json:"cache_dirs,omitempty" yaml:"cache_dirs,omitempty"`

	// Start command
	Start StartCommand `json:"start" yaml:"start"`
}

// DetectionPlan contains detection metadata
type DetectionPlan struct {
	Detected   bool   `json:"detected" yaml:"detected"`
	Language   string `json:"language" yaml:"language"`
	Framework  string `json:"framework" yaml:"framework"`
	Version    string `json:"version,omitempty" yaml:"version,omitempty"`
	Confidence int    `json:"confidence" yaml:"confidence"`
	Provider   string `json:"provider" yaml:"provider"`
}

// BuildPhase represents a phase in the Docker build
type BuildPhase struct {
	Name        string   `json:"name" yaml:"name"`
	DependsOn   []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Commands    []string `json:"commands" yaml:"commands"`
	OnlyInclude []string `json:"only_include,omitempty" yaml:"only_include,omitempty"`
	CacheDirs   []string `json:"cache_dirs,omitempty" yaml:"cache_dirs,omitempty"`
	AptPackages []string `json:"apt_packages,omitempty" yaml:"apt_packages,omitempty"`
}

// CacheDir represents a cache directory for Docker buildkit
type CacheDir struct {
	Path string `json:"path" yaml:"path"`
	ID   string `json:"id" yaml:"id"`
}

// StartCommand represents the container start command
type StartCommand struct {
	Cmd        string `json:"cmd,omitempty" yaml:"cmd,omitempty"`
	Entrypoint string `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty"`
}

var planCmd = &cobra.Command{
	Use:   "plan [path]",
	Short: "Show the build plan without generating files",
	Long: `Output the resolved build plan as JSON or YAML.

This is useful for:
  - Debugging detection issues
  - Understanding what will be generated
  - Integrating with other tools
  - Customizing the build process

Environment Overrides:
  DOCKERIZER_BUILD_CMD    Override build command
  DOCKERIZER_INSTALL_CMD  Override install command
  DOCKERIZER_START_CMD    Override start command
  DOCKERIZER_APT_PKGS     Additional APT packages (comma-separated)

Examples:
  dockerizer plan ./my-project
  dockerizer plan --format yaml ./my-project
  dockerizer plan --output plan.json ./my-project
  DOCKERIZER_START_CMD="npm start" dockerizer plan .`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPlan,
}

func init() {
	planCmd.Flags().String("format", "json", "Output format (json, yaml)")
	planCmd.Flags().StringP("output", "o", "", "Write plan to file instead of stdout")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	format, _ := cmd.Flags().GetString("format")
	outputFile, _ := cmd.Flags().GetString("output")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Scan
	scan, err := scanner.New(scanner.WithIgnoreHidden(false)).Scan(ctx, path)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Detect
	registry := setupRegistry()
	det := detector.New(registry)
	result, err := det.Detect(ctx, scan)
	if err != nil {
		return fmt.Errorf("detection failed: %w", err)
	}

	// Build plan
	plan := buildPlanFromResult(result, scan)

	// Apply environment overrides
	applyEnvOverrides(&plan)

	// Output
	var output []byte
	switch format {
	case "yaml", "yml":
		output, err = yaml.Marshal(plan)
	default:
		output, err = json.MarshalIndent(plan, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, output, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		printInfo("Plan written to %s", outputFile)
	} else {
		fmt.Println(string(output))
	}

	return nil
}

func buildPlanFromResult(result *detector.DetectionResult, scan *scanner.ScanResult) BuildPlan {
	plan := BuildPlan{
		Version:   "1.0",
		Generator: fmt.Sprintf("dockerizer %s", Version),
		Detection: DetectionPlan{
			Detected:   result.Detected,
			Language:   result.Language,
			Framework:  result.Framework,
			Version:    result.Version,
			Confidence: result.Confidence,
			Provider:   result.Provider,
		},
		Variables: result.Variables,
		Phases:    []BuildPhase{},
		CacheDirs: []CacheDir{},
	}

	if !result.Detected {
		return plan
	}

	// Build phases based on language/framework
	switch result.Language {
	case "nodejs":
		plan.Phases = buildNodeJSPhases(result, scan)
		plan.CacheDirs = []CacheDir{
			{Path: "/root/.npm", ID: "npm-cache"},
			{Path: "/app/node_modules", ID: "node-modules"},
		}
	case "python":
		plan.Phases = buildPythonPhases(result, scan)
		plan.CacheDirs = []CacheDir{
			{Path: "/root/.cache/pip", ID: "pip-cache"},
		}
	case "go":
		plan.Phases = buildGoPhases(result, scan)
		plan.CacheDirs = []CacheDir{
			{Path: "/go/pkg/mod", ID: "go-mod-cache"},
			{Path: "/root/.cache/go-build", ID: "go-build-cache"},
		}
	case "rust":
		plan.Phases = buildRustPhases(result, scan)
		plan.CacheDirs = []CacheDir{
			{Path: "/usr/local/cargo/registry", ID: "cargo-registry"},
			{Path: "/app/target", ID: "cargo-target"},
		}
	}

	// Determine start command
	plan.Start = determineStartCommand(result, scan)

	return plan
}

func buildNodeJSPhases(result *detector.DetectionResult, scan *scanner.ScanResult) []BuildPhase {
	phases := []BuildPhase{}

	// Setup phase
	setup := BuildPhase{
		Name:     "setup",
		Commands: []string{"npm ci --only=production"},
	}

	// Check for package manager
	if scan.FileTree.HasFile("pnpm-lock.yaml") {
		setup.Commands = []string{"corepack enable", "pnpm install --frozen-lockfile"}
	} else if scan.FileTree.HasFile("yarn.lock") {
		setup.Commands = []string{"yarn install --frozen-lockfile"}
	} else if scan.FileTree.HasFile("bun.lockb") {
		setup.Commands = []string{"bun install --frozen-lockfile"}
	}

	phases = append(phases, setup)

	// Build phase (if needed)
	if result.Framework == "nextjs" || hasBuildScript(scan) {
		build := BuildPhase{
			Name:      "build",
			DependsOn: []string{"setup"},
			Commands:  []string{"npm run build"},
		}
		phases = append(phases, build)
	}

	return phases
}

func buildPythonPhases(result *detector.DetectionResult, scan *scanner.ScanResult) []BuildPhase {
	phases := []BuildPhase{}

	// Setup phase
	setup := BuildPhase{
		Name:     "setup",
		Commands: []string{"pip install --no-cache-dir -r requirements.txt"},
	}

	// Check for package manager
	if scan.FileTree.HasFile("poetry.lock") {
		setup.Commands = []string{
			"pip install poetry",
			"poetry config virtualenvs.create false",
			"poetry install --no-dev",
		}
	} else if scan.FileTree.HasFile("Pipfile.lock") {
		setup.Commands = []string{
			"pip install pipenv",
			"pipenv install --deploy --system",
		}
	}

	phases = append(phases, setup)

	return phases
}

func buildGoPhases(result *detector.DetectionResult, scan *scanner.ScanResult) []BuildPhase {
	return []BuildPhase{
		{
			Name:     "setup",
			Commands: []string{"go mod download"},
		},
		{
			Name:      "build",
			DependsOn: []string{"setup"},
			Commands:  []string{"go build -o /app/server ."},
		},
	}
}

func buildRustPhases(result *detector.DetectionResult, scan *scanner.ScanResult) []BuildPhase {
	return []BuildPhase{
		{
			Name:     "setup",
			Commands: []string{"cargo fetch"},
		},
		{
			Name:      "build",
			DependsOn: []string{"setup"},
			Commands:  []string{"cargo build --release"},
		},
	}
}

func determineStartCommand(result *detector.DetectionResult, scan *scanner.ScanResult) StartCommand {
	// Check for Procfile first
	for _, kf := range scan.KeyFiles {
		if kf.Path == "Procfile" {
			// Parse Procfile for web process
			lines := parseProcfileLines(kf.Content)
			for _, line := range lines {
				if strings.HasPrefix(line, "web:") {
					return StartCommand{
						Cmd: strings.TrimSpace(strings.TrimPrefix(line, "web:")),
					}
				}
			}
		}
	}

	// Framework-specific defaults
	switch result.Framework {
	case "nextjs":
		return StartCommand{Cmd: "node server.js"}
	case "express":
		return StartCommand{Cmd: "node server.js"}
	case "django":
		return StartCommand{Cmd: "gunicorn config.wsgi:application --bind 0.0.0.0:8000"}
	case "fastapi":
		return StartCommand{Cmd: "uvicorn main:app --host 0.0.0.0 --port 8000"}
	case "flask":
		return StartCommand{Cmd: "gunicorn app:app --bind 0.0.0.0:5000"}
	case "gin", "fiber", "echo":
		return StartCommand{Cmd: "./server"}
	case "actix", "axum":
		return StartCommand{Cmd: "./app"}
	}

	return StartCommand{}
}

func hasBuildScript(scan *scanner.ScanResult) bool {
	if scan.Metadata.PackageJSON != nil {
		if scan.Metadata.PackageJSON.Scripts != nil {
			_, hasBuild := scan.Metadata.PackageJSON.Scripts["build"]
			return hasBuild
		}
	}
	return false
}

func parseProcfileLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

func applyEnvOverrides(plan *BuildPlan) {
	// Override build command
	if cmd := os.Getenv("DOCKERIZER_BUILD_CMD"); cmd != "" {
		for i := range plan.Phases {
			if plan.Phases[i].Name == "build" {
				plan.Phases[i].Commands = []string{cmd}
			}
		}
	}

	// Override install command
	if cmd := os.Getenv("DOCKERIZER_INSTALL_CMD"); cmd != "" {
		for i := range plan.Phases {
			if plan.Phases[i].Name == "setup" {
				plan.Phases[i].Commands = []string{cmd}
			}
		}
	}

	// Override start command
	if cmd := os.Getenv("DOCKERIZER_START_CMD"); cmd != "" {
		plan.Start.Cmd = cmd
	}

	// Add APT packages
	if pkgs := os.Getenv("DOCKERIZER_APT_PKGS"); pkgs != "" {
		aptPkgs := strings.Split(pkgs, ",")
		for i := range plan.Phases {
			plan.Phases[i].AptPackages = append(plan.Phases[i].AptPackages, aptPkgs...)
		}
	}
}
