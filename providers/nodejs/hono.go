package nodejs

import (
	"context"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// HonoProvider detects and generates Dockerfiles for Hono projects
type HonoProvider struct {
	providers.BaseProvider
}

// NewHonoProvider creates a new Hono provider
func NewHonoProvider() *HonoProvider {
	return &HonoProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "hono",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "hono",
			ProviderTemplate:    "nodejs/hono.tmpl",
			ProviderDescription: "Hono ultrafast web framework",
			ProviderURL:         "https://hono.dev",
		},
	}
}

// Detect checks if the repository is a Hono project
func (p *HonoProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for hono dependency (required)
	if pkg.HasDependency("hono") {
		score += 50
	} else {
		return 0, nil, nil // Not Hono
	}

	// Check for @hono/node-server (Node.js adapter)
	if pkg.HasDependency("@hono/node-server") {
		score += 20
		vars["hasNodeAdapter"] = true
	}

	// Check for common entry points
	if scan.FileTree.HasFile("src/index.ts") || scan.FileTree.HasFile("src/index.js") {
		score += 10
	}
	if scan.FileTree.HasFile("index.ts") || scan.FileTree.HasFile("index.js") {
		score += 10
	}

	// Check for TypeScript
	if scan.FileTree.HasFile("tsconfig.json") {
		score += 5
		vars["typescript"] = true
	}

	// Check for Bun (Hono is often used with Bun)
	var pm string
	if scan.FileTree.HasFile("bun.lockb") {
		vars["runtime"] = "bun"
		pm = "bun"
	} else {
		vars["runtime"] = "node"
		pm = detectPackageManager(scan)
	}
	vars["packageManager"] = pm
	vars["hasLockFile"] = hasLockFile(scan, pm)

	// Detect Node version
	vars["nodeVersion"] = p.DetectVersion(scan)

	// Check for start script
	if pkg.HasScript("start") {
		vars["hasStartScript"] = true
	}

	// Detect main entry point
	if pkg.Main != "" {
		vars["mainEntry"] = pkg.Main
	} else if scan.FileTree.HasFile("src/index.ts") {
		vars["mainEntry"] = "src/index.ts"
	} else if scan.FileTree.HasFile("src/index.js") {
		vars["mainEntry"] = "src/index.js"
	} else if scan.FileTree.HasFile("index.ts") {
		vars["mainEntry"] = "index.ts"
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
func (p *HonoProvider) DetectVersion(scan *scanner.ScanResult) string {
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
