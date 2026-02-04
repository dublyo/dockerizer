package nodejs

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// AstroProvider detects and generates Dockerfiles for Astro projects
type AstroProvider struct {
	providers.BaseProvider
}

// NewAstroProvider creates a new Astro provider
func NewAstroProvider() *AstroProvider {
	return &AstroProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "astro",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "astro",
			ProviderTemplate:    "nodejs/astro.tmpl",
			ProviderDescription: "Astro web framework for content-driven websites",
			ProviderURL:         "https://astro.build",
		},
	}
}

// Detect checks if the repository is an Astro project
func (p *AstroProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for astro dependency (required)
	if pkg.HasDependency("astro") {
		score += 50
	} else {
		return 0, nil, nil // Not Astro
	}

	// Check for astro.config.mjs or astro.config.ts
	if scan.FileTree.HasFile("astro.config.mjs") || scan.FileTree.HasFile("astro.config.ts") || scan.FileTree.HasFile("astro.config.js") {
		score += 20
	}

	// Check for src/pages directory (Astro convention)
	if scan.FileTree.HasDir("src/pages") {
		score += 10
	}

	// Check for src/layouts directory
	if scan.FileTree.HasDir("src/layouts") {
		score += 5
	}

	// Check for src/components directory
	if scan.FileTree.HasDir("src/components") {
		score += 5
	}

	// Detect output mode from astro.config
	vars["outputMode"] = "static" // default
	configFiles := []string{"astro.config.mjs", "astro.config.ts", "astro.config.js"}
	for _, configFile := range configFiles {
		if scan.FileTree.HasFile(configFile) {
			data, err := scan.ReadFile(configFile)
			if err == nil {
				content := string(data)
				if strings.Contains(content, "output: 'server'") || strings.Contains(content, "output: \"server\"") {
					vars["outputMode"] = "server"
					score += 5
				} else if strings.Contains(content, "output: 'hybrid'") || strings.Contains(content, "output: \"hybrid\"") {
					vars["outputMode"] = "hybrid"
					score += 5
				}
				// Check for Node adapter
				if strings.Contains(content, "@astrojs/node") {
					vars["hasNodeAdapter"] = true
				}
			}
			break
		}
	}

	// Detect package manager
	vars["packageManager"] = detectPackageManager(scan)

	// Detect Node version
	vars["nodeVersion"] = p.DetectVersion(scan)

	// Check for TypeScript
	if scan.FileTree.HasFile("tsconfig.json") {
		vars["typescript"] = true
	}

	// Detect port
	vars["port"] = detectPort(scan, "4321")

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Node.js version to use
func (p *AstroProvider) DetectVersion(scan *scanner.ScanResult) string {
	if scan.Metadata.PackageJSON == nil {
		return "20"
	}

	pkg := scan.Metadata.PackageJSON

	if pkg.Engines.Node != "" {
		return parseNodeVersion(pkg.Engines.Node)
	}

	if scan.FileTree.HasFile(".nvmrc") {
		data, err := scan.ReadFile(".nvmrc")
		if err == nil {
			return parseNodeVersion(string(data))
		}
	}

	return "20"
}
