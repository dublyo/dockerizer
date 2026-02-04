package nodejs

import (
	"context"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// NestJSProvider detects and generates Dockerfiles for NestJS projects
type NestJSProvider struct {
	providers.BaseProvider
}

// NewNestJSProvider creates a new NestJS provider
func NewNestJSProvider() *NestJSProvider {
	return &NestJSProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "nestjs",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "nestjs",
			ProviderTemplate:    "nodejs/nestjs.tmpl",
			ProviderDescription: "NestJS progressive Node.js framework",
			ProviderURL:         "https://nestjs.com",
		},
	}
}

// Detect checks if the repository is a NestJS project
func (p *NestJSProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for @nestjs/core dependency (required)
	if pkg.HasDependency("@nestjs/core") {
		score += 50
	} else {
		return 0, nil, nil // Not NestJS
	}

	// Check for @nestjs/common
	if pkg.HasDependency("@nestjs/common") {
		score += 15
	}

	// Check for @nestjs/platform-express or @nestjs/platform-fastify
	if pkg.HasDependency("@nestjs/platform-express") {
		score += 10
		vars["platform"] = "express"
	} else if pkg.HasDependency("@nestjs/platform-fastify") {
		score += 10
		vars["platform"] = "fastify"
	}

	// Check for nest-cli.json
	if scan.FileTree.HasFile("nest-cli.json") {
		score += 15
	}

	// Check for src/main.ts (NestJS convention)
	if scan.FileTree.HasFile("src/main.ts") {
		score += 10
		vars["mainFile"] = "src/main.ts"
	}

	// TypeScript is standard in NestJS
	if pkg.HasDependency("typescript") || scan.FileTree.HasFile("tsconfig.json") {
		vars["typescript"] = true
	}

	// Detect package manager
	vars["packageManager"] = detectPackageManager(scan)

	// Detect Node version
	vars["nodeVersion"] = p.DetectVersion(scan)

	// Check for scripts
	if pkg.HasScript("build") {
		vars["buildScript"] = "build"
	}
	if pkg.HasScript("start:prod") {
		vars["startScript"] = "start:prod"
	} else if pkg.HasScript("start") {
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
func (p *NestJSProvider) DetectVersion(scan *scanner.ScanResult) string {
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
