// Package nodejs provides detection for Node.js frameworks.
package nodejs

import (
	"context"
	"regexp"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// NextJSProvider detects and generates Dockerfiles for Next.js projects
type NextJSProvider struct {
	providers.BaseProvider
}

// NewNextJSProvider creates a new Next.js provider
func NewNextJSProvider() *NextJSProvider {
	return &NextJSProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "nextjs",
			ProviderLanguage:    "nodejs",
			ProviderFramework:   "nextjs",
			ProviderTemplate:    "nodejs/nextjs.tmpl",
			ProviderDescription: "Next.js React Framework",
			ProviderURL:         "https://nextjs.org",
		},
	}
}

// Detect checks if the repository is a Next.js project
func (p *NextJSProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have package.json
	if scan.Metadata.PackageJSON == nil {
		return 0, nil, nil
	}

	pkg := scan.Metadata.PackageJSON

	// Check for next dependency (required)
	if pkg.HasDependency("next") {
		score += 40
	} else {
		return 0, nil, nil // Not Next.js
	}

	// Check for next.config.* file
	configFiles := []string{"next.config.js", "next.config.ts", "next.config.mjs"}
	for _, cf := range configFiles {
		if scan.FileTree.HasFile(cf) {
			score += 30
			break
		}
	}

	// Check for app/ or pages/ directory (Next.js routing)
	hasAppDir := scan.FileTree.HasDir("app") || scan.FileTree.HasDir("src/app")
	hasPagesDir := scan.FileTree.HasDir("pages") || scan.FileTree.HasDir("src/pages")
	if hasAppDir {
		score += 20
		vars["routingMode"] = "app"
	} else if hasPagesDir {
		score += 15
		vars["routingMode"] = "pages"
	}

	// Check for React dependency (expected with Next.js)
	if pkg.HasDependency("react") {
		score += 10
	}

	// Detect package manager
	vars["packageManager"] = detectPackageManager(scan)

	// Detect Node version
	vars["nodeVersion"] = p.DetectVersion(scan)

	// Check for TypeScript
	if pkg.HasDependency("typescript") || scan.FileTree.HasFile("tsconfig.json") {
		vars["typescript"] = true
	}

	// Check for standalone output (for Docker optimization)
	if scan.FileTree.HasFile("next.config.js") || scan.FileTree.HasFile("next.config.mjs") {
		content, err := scan.ReadFile("next.config.js")
		if err != nil {
			content, _ = scan.ReadFile("next.config.mjs")
		}
		if strings.Contains(string(content), "standalone") {
			vars["standalone"] = true
		}
	}

	// Check for common scripts
	if pkg.HasScript("build") {
		vars["buildScript"] = "build"
	}
	if pkg.HasScript("start") {
		vars["startScript"] = "start"
	}

	// Detect port from environment or common patterns
	vars["port"] = detectPort(scan, "3000")

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Node.js version to use
func (p *NextJSProvider) DetectVersion(scan *scanner.ScanResult) string {
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
			version := strings.TrimSpace(string(data))
			return parseNodeVersion(version)
		}
	}

	// Check .node-version
	if scan.FileTree.HasFile(".node-version") {
		data, err := scan.ReadFile(".node-version")
		if err == nil {
			version := strings.TrimSpace(string(data))
			return parseNodeVersion(version)
		}
	}

	// Default to Node 20 LTS
	return "20"
}

// detectPackageManager determines which package manager to use
func detectPackageManager(scan *scanner.ScanResult) string {
	// Check for lock files in order of preference
	if scan.FileTree.HasFile("pnpm-lock.yaml") {
		return "pnpm"
	}
	if scan.FileTree.HasFile("yarn.lock") {
		return "yarn"
	}
	if scan.FileTree.HasFile("bun.lockb") {
		return "bun"
	}
	if scan.FileTree.HasFile("package-lock.json") {
		return "npm"
	}

	// Check packageManager field in package.json
	if scan.Metadata.PackageJSON != nil && scan.Metadata.PackageJSON.PackageManager != "" {
		pm := scan.Metadata.PackageJSON.PackageManager
		if strings.HasPrefix(pm, "pnpm") {
			return "pnpm"
		}
		if strings.HasPrefix(pm, "yarn") {
			return "yarn"
		}
		if strings.HasPrefix(pm, "bun") {
			return "bun"
		}
	}

	// Default to npm
	return "npm"
}

// detectPort determines the port the application will listen on
func detectPort(scan *scanner.ScanResult, defaultPort string) string {
	// Check for existing .env file
	if scan.FileTree.HasFile(".env") {
		data, err := scan.ReadFile(".env")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PORT=") {
					return strings.TrimPrefix(line, "PORT=")
				}
			}
		}
	}

	return defaultPort
}

// parseNodeVersion extracts the major version from a version string
func parseNodeVersion(version string) string {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// Handle ranges like ">=18.0.0" or "^20.0.0"
	re := regexp.MustCompile(`(\d+)`)
	matches := re.FindString(version)
	if matches != "" {
		return matches
	}

	return "20"
}
