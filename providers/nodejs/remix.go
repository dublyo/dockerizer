package nodejs

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// RemixProvider detects and generates Dockerfiles for Remix projects
type RemixProvider struct {
	providers.BaseProvider
}

// NewRemixProvider creates a new Remix provider
func NewRemixProvider() *RemixProvider {
	return &RemixProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "remix",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "remix",
			ProviderTemplate:    "nodejs/remix.tmpl",
			ProviderDescription: "Remix full-stack React framework",
			ProviderURL:         "https://remix.run",
		},
	}
}

// Detect checks if the repository is a Remix project
func (p *RemixProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for @remix-run/node or @remix-run/react dependency (required)
	hasRemix := pkg.HasDependency("@remix-run/node") ||
		pkg.HasDependency("@remix-run/react") ||
		pkg.HasDependency("@remix-run/serve")

	if hasRemix {
		score += 50
	} else {
		return 0, nil, nil // Not Remix
	}

	// Check for remix.config.js or remix.config.ts
	if scan.FileTree.HasFile("remix.config.js") || scan.FileTree.HasFile("remix.config.ts") {
		score += 20
	}

	// Check for vite.config.ts with Remix (Remix v2+)
	if scan.FileTree.HasFile("vite.config.ts") || scan.FileTree.HasFile("vite.config.js") {
		data, err := scan.ReadFile("vite.config.ts")
		if err != nil {
			data, _ = scan.ReadFile("vite.config.js")
		}
		if strings.Contains(string(data), "@remix-run/dev") {
			score += 15
			vars["usesVite"] = true
		}
	}

	// Check for app directory (Remix convention)
	if scan.FileTree.HasDir("app") {
		score += 10
	}

	// Check for root.tsx (Remix entry point)
	if scan.FileTree.HasFile("app/root.tsx") || scan.FileTree.HasFile("app/root.jsx") {
		score += 10
	}

	// Check for routes directory
	if scan.FileTree.HasDir("app/routes") {
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

	// Detect port
	vars["port"] = detectPort(scan, "3000")

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Node.js version to use
func (p *RemixProvider) DetectVersion(scan *scanner.ScanResult) string {
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
