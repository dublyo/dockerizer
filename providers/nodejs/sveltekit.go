package nodejs

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// SvelteKitProvider detects and generates Dockerfiles for SvelteKit projects
type SvelteKitProvider struct {
	providers.BaseProvider
}

// NewSvelteKitProvider creates a new SvelteKit provider
func NewSvelteKitProvider() *SvelteKitProvider {
	return &SvelteKitProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "sveltekit",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "sveltekit",
			ProviderTemplate:    "nodejs/sveltekit.tmpl",
			ProviderDescription: "SvelteKit full-stack framework",
			ProviderURL:         "https://kit.svelte.dev",
		},
	}
}

// Detect checks if the repository is a SvelteKit project
func (p *SvelteKitProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for @sveltejs/kit dependency (required)
	// HasDependency checks both dependencies and devDependencies
	if pkg.HasDependency("@sveltejs/kit") {
		score += 50
	} else {
		return 0, nil, nil // Not SvelteKit
	}

	// Check for svelte.config.js
	if scan.FileTree.HasFile("svelte.config.js") || scan.FileTree.HasFile("svelte.config.ts") {
		score += 20
	}

	// Check for src/routes directory (SvelteKit convention)
	if scan.FileTree.HasDir("src/routes") {
		score += 15
	}

	// Check for src/app.html
	if scan.FileTree.HasFile("src/app.html") {
		score += 5
	}

	// Check for +page.svelte files
	if scan.FileTree.HasFile("src/routes/+page.svelte") {
		score += 5
	}

	// Detect adapter from svelte.config.js
	vars["adapter"] = "node" // default
	configFiles := []string{"svelte.config.js", "svelte.config.ts"}
	for _, configFile := range configFiles {
		if scan.FileTree.HasFile(configFile) {
			data, err := scan.ReadFile(configFile)
			if err == nil {
				content := string(data)
				if strings.Contains(content, "@sveltejs/adapter-node") {
					vars["adapter"] = "node"
				} else if strings.Contains(content, "@sveltejs/adapter-static") {
					vars["adapter"] = "static"
				} else if strings.Contains(content, "@sveltejs/adapter-auto") {
					vars["adapter"] = "auto"
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
	vars["port"] = detectPort(scan, "3000")

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Node.js version to use
func (p *SvelteKitProvider) DetectVersion(scan *scanner.ScanResult) string {
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
