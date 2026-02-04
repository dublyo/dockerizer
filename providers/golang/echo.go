package golang

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// EchoProvider detects Echo projects
type EchoProvider struct {
	providers.BaseProvider
}

// NewEchoProvider creates a new Echo provider
func NewEchoProvider() *EchoProvider {
	return &EchoProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "echo",
			ProviderLanguage:    "go",
			ProviderFramework:   "echo",
			ProviderTemplate:    "go/echo.tmpl",
			ProviderDescription: "Echo high performance web framework",
			ProviderURL:         "https://echo.labstack.com",
		},
	}
}

// Detect checks if the repository is an Echo project
func (p *EchoProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have go.mod
	if scan.Metadata.GoMod == nil {
		return 0, nil, nil
	}

	// Check for labstack/echo in go.mod
	for _, req := range scan.Metadata.GoMod.Require {
		if strings.Contains(req, "github.com/labstack/echo") {
			score += 60
			break
		}
	}

	// Check for echo import in .go files
	goFiles := scan.FileTree.FilesWithExtension(".go")
	for _, gf := range goFiles {
		data, err := scan.ReadFile(gf)
		if err == nil {
			content := string(data)
			if strings.Contains(content, `"github.com/labstack/echo`) {
				score += 20
				break
			}
		}
	}

	// Check for main.go
	if scan.FileTree.HasFile("main.go") {
		score += 10
	}

	if score == 0 {
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
func (p *EchoProvider) DetectVersion(scan *scanner.ScanResult) string {
	return detectGoVersion(scan)
}
