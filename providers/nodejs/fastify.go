package nodejs

import (
	"context"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// FastifyProvider detects and generates Dockerfiles for Fastify projects
type FastifyProvider struct {
	providers.BaseProvider
}

// NewFastifyProvider creates a new Fastify provider
func NewFastifyProvider() *FastifyProvider {
	return &FastifyProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "fastify",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "fastify",
			ProviderTemplate:    "nodejs/fastify.tmpl",
			ProviderDescription: "Fastify web framework",
			ProviderURL:         "https://fastify.io",
		},
	}
}

// Detect checks if the repository is a Fastify project
func (p *FastifyProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for fastify dependency (required)
	if pkg.HasDependency("fastify") {
		score += 50
	} else {
		return 0, nil, nil // Not Fastify
	}

	// Check for main entry point
	mainFile := pkg.Main
	if mainFile == "" {
		entryPoints := []string{"index.js", "app.js", "server.js", "src/index.js", "src/app.js", "src/server.js", "src/index.ts", "src/app.ts", "src/server.ts"}
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

	// Check for common Fastify plugins
	plugins := []string{
		"@fastify/cors",
		"@fastify/helmet",
		"@fastify/swagger",
		"@fastify/jwt",
		"@fastify/cookie",
		"@fastify/session",
		"@fastify/rate-limit",
		"@fastify/autoload",
		"@fastify/sensible",
		"fastify-plugin",
	}
	for _, plugin := range plugins {
		if pkg.HasDependency(plugin) {
			score += 5
		}
	}

	// Check for TypeScript
	if pkg.HasDependency("typescript") || scan.FileTree.HasFile("tsconfig.json") {
		vars["typescript"] = true
		score += 5
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

	// Check for Fastify CLI
	if pkg.HasDependency("fastify-cli") {
		vars["hasCLI"] = true
		score += 5
	}

	// Check for engines specification
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
func (p *FastifyProvider) DetectVersion(scan *scanner.ScanResult) string {
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
