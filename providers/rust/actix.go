// Package rust provides detection for Rust frameworks.
package rust

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// ActixProvider detects Actix-web projects
type ActixProvider struct {
	providers.BaseProvider
}

// NewActixProvider creates a new Actix provider
func NewActixProvider() *ActixProvider {
	return &ActixProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "actix",
			ProviderLanguage:    "rust",
			ProviderFramework:   "actix-web",
			ProviderTemplate:    "rust/actix.tmpl",
			ProviderDescription: "Actix Web framework",
			ProviderURL:         "https://actix.rs",
		},
	}
}

// Detect checks if the repository is an Actix project
func (p *ActixProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have Cargo.toml
	if scan.Metadata.CargoToml == nil {
		return 0, nil, nil
	}

	// Check for actix-web in dependencies
	for _, dep := range scan.Metadata.CargoToml.Dependencies {
		if strings.Contains(dep, "actix-web") {
			score += 60
			break
		}
	}

	// Check Cargo.toml content for actix-web
	if scan.FileTree.HasFile("Cargo.toml") {
		data, err := scan.ReadFile("Cargo.toml")
		if err == nil && strings.Contains(string(data), "actix-web") {
			score += 20
		}
	}

	// Check for src/main.rs
	if scan.FileTree.HasFile("src/main.rs") {
		score += 10
	}

	if score == 0 {
		return 0, nil, nil
	}

	vars["rustVersion"] = p.DetectVersion(scan)
	vars["projectName"] = scan.Metadata.CargoToml.Name
	vars["port"] = "8080"

	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Rust version to use
func (p *ActixProvider) DetectVersion(scan *scanner.ScanResult) string {
	// Check rust-toolchain.toml or rust-toolchain
	if scan.FileTree.HasFile("rust-toolchain.toml") {
		data, err := scan.ReadFile("rust-toolchain.toml")
		if err == nil {
			content := string(data)
			if strings.Contains(content, "stable") {
				return "1.75"
			}
		}
	}
	if scan.FileTree.HasFile("rust-toolchain") {
		data, err := scan.ReadFile("rust-toolchain")
		if err == nil {
			version := strings.TrimSpace(string(data))
			if version != "" && !strings.Contains(version, "stable") {
				return version
			}
		}
	}
	// Default to latest stable
	return "1.75"
}
