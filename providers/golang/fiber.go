package golang

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// FiberProvider detects Fiber projects
type FiberProvider struct {
	providers.BaseProvider
}

// NewFiberProvider creates a new Fiber provider
func NewFiberProvider() *FiberProvider {
	return &FiberProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "fiber",
			ProviderLanguage:    "go",
			ProviderFramework:   "fiber",
			ProviderTemplate:    "go/fiber.tmpl",
			ProviderDescription: "Fiber Express-inspired web framework",
			ProviderURL:         "https://gofiber.io",
		},
	}
}

// Detect checks if the repository is a Fiber project
func (p *FiberProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have go.mod
	if scan.Metadata.GoMod == nil {
		return 0, nil, nil
	}

	// Check for gofiber/fiber in go.mod
	for _, req := range scan.Metadata.GoMod.Require {
		if strings.Contains(req, "github.com/gofiber/fiber") {
			score += 60
			break
		}
	}

	// Check for fiber import in .go files
	goFiles := scan.FileTree.FilesWithExtension(".go")
	for _, gf := range goFiles {
		data, err := scan.ReadFile(gf)
		if err == nil {
			content := string(data)
			if strings.Contains(content, `"github.com/gofiber/fiber`) {
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
func (p *FiberProvider) DetectVersion(scan *scanner.ScanResult) string {
	return detectGoVersion(scan)
}
