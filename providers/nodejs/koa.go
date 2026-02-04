package nodejs

import (
	"context"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// KoaProvider detects and generates Dockerfiles for Koa.js projects
type KoaProvider struct {
	providers.BaseProvider
}

// NewKoaProvider creates a new Koa.js provider
func NewKoaProvider() *KoaProvider {
	return &KoaProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "koa",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "koa",
			ProviderTemplate:    "nodejs/koa.tmpl",
			ProviderDescription: "Koa.js web framework",
			ProviderURL:         "https://koajs.com",
		},
	}
}

// Detect checks if the repository is a Koa.js project
func (p *KoaProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for koa dependency (required)
	if pkg.HasDependency("koa") {
		score += 50
	} else {
		return 0, nil, nil // Not Koa
	}

	// Check for koa-router
	if pkg.HasDependency("koa-router") || pkg.HasDependency("@koa/router") {
		score += 15
	}

	// Check for koa-bodyparser
	if pkg.HasDependency("koa-bodyparser") || pkg.HasDependency("@koa/bodyparser") {
		score += 10
	}

	// Check for common entry points
	if scan.FileTree.HasFile("app.js") || scan.FileTree.HasFile("app.ts") {
		score += 10
	}
	if scan.FileTree.HasFile("server.js") || scan.FileTree.HasFile("server.ts") {
		score += 10
	}
	if scan.FileTree.HasFile("src/app.js") || scan.FileTree.HasFile("src/app.ts") {
		score += 10
	}
	if scan.FileTree.HasFile("src/index.js") || scan.FileTree.HasFile("src/index.ts") {
		score += 5
	}

	// Detect package manager
	pm := detectPackageManager(scan)
	vars["packageManager"] = pm
	vars["hasLockFile"] = hasLockFile(scan, pm)

	// Detect Node version
	vars["nodeVersion"] = p.DetectVersion(scan)

	// Check for TypeScript
	if scan.FileTree.HasFile("tsconfig.json") {
		vars["typescript"] = true
	}

	// Check for start script
	if pkg.HasScript("start") {
		vars["hasStartScript"] = true
	}

	// Detect main entry point
	if pkg.Main != "" {
		vars["mainEntry"] = pkg.Main
	} else if scan.FileTree.HasFile("src/index.js") {
		vars["mainEntry"] = "src/index.js"
	} else if scan.FileTree.HasFile("app.js") {
		vars["mainEntry"] = "app.js"
	} else if scan.FileTree.HasFile("server.js") {
		vars["mainEntry"] = "server.js"
	} else {
		vars["mainEntry"] = "index.js"
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
func (p *KoaProvider) DetectVersion(scan *scanner.ScanResult) string {
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
