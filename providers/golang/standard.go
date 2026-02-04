package golang

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// StandardProvider detects standard Go HTTP projects
type StandardProvider struct {
	providers.BaseProvider
}

// NewStandardProvider creates a new standard Go provider
func NewStandardProvider() *StandardProvider {
	return &StandardProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "go-standard",
			ProviderLanguage:    "go",
			ProviderFramework:   "standard",
			ProviderTemplate:    "go/standard.tmpl",
			ProviderDescription: "Go standard library HTTP server",
			ProviderURL:         "https://go.dev",
		},
	}
}

// Detect checks if the repository is a standard Go HTTP project
func (p *StandardProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have go.mod
	if scan.Metadata.GoMod == nil {
		return 0, nil, nil
	}

	score += 30 // Has go.mod

	// Check for net/http import in .go files (standard library)
	goFiles := scan.FileTree.FilesWithExtension(".go")
	for _, gf := range goFiles {
		data, err := scan.ReadFile(gf)
		if err == nil {
			content := string(data)
			if strings.Contains(content, `"net/http"`) {
				score += 30
				break
			}
		}
	}

	// Check for main.go
	if scan.FileTree.HasFile("main.go") {
		score += 20
	}

	// Check for cmd directory structure
	if scan.FileTree.HasDir("cmd") {
		score += 10
	}

	if score < 50 { // Need at least go.mod and main.go or net/http
		return 0, nil, nil
	}

	vars["goVersion"] = p.DetectVersion(scan)
	vars["moduleName"] = scan.Metadata.GoMod.Module
	vars["port"] = detectGoPort(scan)
	vars["mainPath"] = detectMainPath(scan)

	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Go version
func (p *StandardProvider) DetectVersion(scan *scanner.ScanResult) string {
	return detectGoVersion(scan)
}
