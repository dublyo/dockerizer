package nodejs

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// NuxtProvider detects and generates Dockerfiles for Nuxt.js projects
type NuxtProvider struct {
	providers.BaseProvider
}

// NewNuxtProvider creates a new Nuxt.js provider
func NewNuxtProvider() *NuxtProvider {
	return &NuxtProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "nuxt",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "nuxt",
			ProviderTemplate:    "nodejs/nuxt.tmpl",
			ProviderDescription: "Nuxt.js Vue.js meta-framework",
			ProviderURL:         "https://nuxt.com",
		},
	}
}

// Detect checks if the repository is a Nuxt.js project
func (p *NuxtProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for nuxt dependency (required)
	if pkg.HasDependency("nuxt") {
		score += 50
	} else {
		return 0, nil, nil // Not Nuxt
	}

	// Check for nuxt.config.ts or nuxt.config.js
	if scan.FileTree.HasFile("nuxt.config.ts") {
		score += 25
		vars["typescript"] = true
	} else if scan.FileTree.HasFile("nuxt.config.js") {
		score += 25
	}

	// Check for .nuxt directory (development build output)
	if scan.FileTree.HasDir(".nuxt") {
		score += 5
	}

	// Check for Nuxt 3 specific directories
	if scan.FileTree.HasDir("server") {
		score += 5
		vars["hasServer"] = true
	}

	// Check for pages directory (file-based routing)
	if scan.FileTree.HasDir("pages") {
		score += 5
	}

	// Check for app.vue (Nuxt 3 entry point)
	if scan.FileTree.HasFile("app.vue") {
		score += 5
		vars["nuxtVersion"] = "3"
	}

	// Detect Nuxt version from package.json
	if version, ok := pkg.Dependencies["nuxt"]; ok {
		if strings.HasPrefix(version, "3") || strings.HasPrefix(version, "^3") {
			vars["nuxtVersion"] = "3"
		} else if strings.HasPrefix(version, "2") || strings.HasPrefix(version, "^2") {
			vars["nuxtVersion"] = "2"
		}
	}

	// Detect package manager
	pm := detectPackageManager(scan)
	vars["packageManager"] = pm
	vars["hasLockFile"] = hasLockFile(scan, pm)

	// Detect Node version
	vars["nodeVersion"] = p.DetectVersion(scan)

	// Check for scripts
	if pkg.HasScript("build") {
		vars["buildScript"] = "build"
	}
	if pkg.HasScript("start") {
		vars["startScript"] = "start"
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
func (p *NuxtProvider) DetectVersion(scan *scanner.ScanResult) string {
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
