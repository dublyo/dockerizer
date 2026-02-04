// Package golang provides detection for Go frameworks.
package golang

import (
	"context"
	"regexp"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// GinProvider detects Gin projects
type GinProvider struct {
	providers.BaseProvider
}

// NewGinProvider creates a new Gin provider
func NewGinProvider() *GinProvider {
	return &GinProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "gin",
			ProviderLanguage:    "go",
			ProviderFramework:   "gin",
			ProviderTemplate:    "go/gin.tmpl",
			ProviderDescription: "Gin HTTP web framework",
			ProviderURL:         "https://gin-gonic.com",
		},
	}
}

// Detect checks if the repository is a Gin project
func (p *GinProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have go.mod
	if scan.Metadata.GoMod == nil {
		return 0, nil, nil
	}

	// Check for gin-gonic/gin in go.mod
	for _, req := range scan.Metadata.GoMod.Require {
		if strings.Contains(req, "github.com/gin-gonic/gin") {
			score += 60
			break
		}
	}

	// Check for gin import in .go files
	goFiles := scan.FileTree.FilesWithExtension(".go")
	for _, gf := range goFiles {
		data, err := scan.ReadFile(gf)
		if err == nil {
			content := string(data)
			if strings.Contains(content, `"github.com/gin-gonic/gin"`) {
				score += 20
				break
			}
		}
	}

	// Check for main.go
	if scan.FileTree.HasFile("main.go") || scan.FileTree.HasFile("cmd/main.go") || scan.FileTree.HasFile("cmd/server/main.go") {
		score += 10
	}

	if score == 0 {
		return 0, nil, nil
	}

	// Set variables
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
func (p *GinProvider) DetectVersion(scan *scanner.ScanResult) string {
	return detectGoVersion(scan)
}

func detectGoVersion(scan *scanner.ScanResult) string {
	if scan.Metadata.GoMod != nil && scan.Metadata.GoMod.Go != "" {
		return scan.Metadata.GoMod.Go
	}
	return "1.22" // Default to recent stable
}

func detectGoPort(scan *scanner.ScanResult) string {
	// Check for common port patterns in main.go or config files
	mainFiles := []string{"main.go", "cmd/main.go", "cmd/server/main.go"}
	for _, mf := range mainFiles {
		if scan.FileTree.HasFile(mf) {
			data, err := scan.ReadFile(mf)
			if err == nil {
				content := string(data)
				// Look for :8080, :3000, etc.
				re := regexp.MustCompile(`:(\d{4})`)
				matches := re.FindStringSubmatch(content)
				if len(matches) > 1 {
					return matches[1]
				}
			}
		}
	}
	return "8080" // Go default
}

func detectMainPath(scan *scanner.ScanResult) string {
	if scan.FileTree.HasFile("cmd/server/main.go") {
		return "./cmd/server"
	}
	if scan.FileTree.HasFile("cmd/main.go") {
		return "./cmd"
	}
	if scan.FileTree.HasFile("cmd/api/main.go") {
		return "./cmd/api"
	}
	return "."
}
