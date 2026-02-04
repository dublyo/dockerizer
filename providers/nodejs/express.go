package nodejs

import (
	"context"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// ExpressProvider detects and generates Dockerfiles for Express.js projects
type ExpressProvider struct {
	providers.BaseProvider
}

// NewExpressProvider creates a new Express.js provider
func NewExpressProvider() *ExpressProvider {
	return &ExpressProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "express",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "express",
			ProviderTemplate:    "nodejs/express.tmpl",
			ProviderDescription: "Express.js web framework",
			ProviderURL:         "https://expressjs.com",
		},
	}
}

// Detect checks if the repository is an Express.js project
func (p *ExpressProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for express dependency (required)
	if pkg.HasDependency("express") {
		score += 50
	} else {
		return 0, nil, nil // Not Express
	}

	// Check for common Express patterns
	// Main entry point
	mainFile := pkg.Main
	if mainFile == "" {
		// Check for common entry points
		entryPoints := []string{"index.js", "app.js", "server.js", "src/index.js", "src/app.js", "src/server.js"}
		for _, ep := range entryPoints {
			if scan.FileTree.HasFile(ep) {
				mainFile = ep
				break
			}
		}
	}
	if mainFile != "" {
		score += 20
		vars["mainFile"] = mainFile
	}

	// Check for common Express middleware
	middlewarePackages := []string{
		"body-parser",
		"cors",
		"helmet",
		"morgan",
		"cookie-parser",
		"express-session",
		"passport",
		"compression",
		"express-validator",
	}
	for _, middleware := range middlewarePackages {
		if pkg.HasDependency(middleware) {
			score += 5
		}
	}

	// Check for TypeScript
	if pkg.HasDependency("typescript") || scan.FileTree.HasFile("tsconfig.json") {
		vars["typescript"] = true
		score += 5
		// Check for ts-node or tsx
		if pkg.HasDependency("ts-node") || pkg.HasDependency("tsx") {
			vars["tsRunner"] = true
		}
	}

	// Detect package manager
	pm := detectPackageManager(scan)
	vars["packageManager"] = pm
	vars["hasLockFile"] = hasLockFile(scan, pm)

	// Detect Node version
	vars["nodeVersion"] = p.DetectVersion(scan)

	// Check for common scripts
	if pkg.HasScript("start") {
		vars["startScript"] = "start"
		score += 10
	}
	if pkg.HasScript("build") {
		vars["buildScript"] = "build"
		score += 5
	}

	// Detect port
	vars["port"] = detectPort(scan, "3000")

	// Check for module type
	if pkg.Type == "module" {
		vars["esm"] = true
	}

	// Check for engines specification (production ready)
	if pkg.Engines.Node != "" {
		score += 5
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Node.js version to use
func (p *ExpressProvider) DetectVersion(scan *scanner.ScanResult) string {
	if scan.Metadata.PackageJSON == nil {
		return "20"
	}

	pkg := scan.Metadata.PackageJSON

	// Check engines.node
	if pkg.Engines.Node != "" {
		return parseNodeVersion(pkg.Engines.Node)
	}

	// Check .nvmrc
	if scan.FileTree.HasFile(".nvmrc") {
		data, err := scan.ReadFile(".nvmrc")
		if err == nil {
			return parseNodeVersion(string(data))
		}
	}

	// Default to Node 20 LTS
	return "20"
}
