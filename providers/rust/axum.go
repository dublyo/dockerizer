package rust

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// AxumProvider detects Axum projects
type AxumProvider struct {
	providers.BaseProvider
}

// NewAxumProvider creates a new Axum provider
func NewAxumProvider() *AxumProvider {
	return &AxumProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "axum",
			ProviderLanguage:    "rust",
			ProviderFramework:   "axum",
			ProviderTemplate:    "rust/axum.tmpl",
			ProviderDescription: "Axum web framework",
			ProviderURL:         "https://github.com/tokio-rs/axum",
		},
	}
}

// Detect checks if the repository is an Axum project
func (p *AxumProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	if scan.Metadata.CargoToml == nil {
		return 0, nil, nil
	}

	// Check Cargo.toml for axum
	if scan.FileTree.HasFile("Cargo.toml") {
		data, err := scan.ReadFile("Cargo.toml")
		if err == nil && strings.Contains(string(data), "axum") {
			score += 70
		}
	}

	if scan.FileTree.HasFile("src/main.rs") {
		score += 10
	}

	if score == 0 {
		return 0, nil, nil
	}

	vars["rustVersion"] = p.DetectVersion(scan)
	vars["projectName"] = scan.Metadata.CargoToml.Name
	vars["port"] = "3000"

	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Rust edition
func (p *AxumProvider) DetectVersion(scan *scanner.ScanResult) string {
	if scan.Metadata.CargoToml != nil && scan.Metadata.CargoToml.Edition != "" {
		return scan.Metadata.CargoToml.Edition
	}
	return "2021"
}
