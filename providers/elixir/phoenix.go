package elixir

import (
	"context"
	"regexp"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// PhoenixProvider detects and generates Dockerfiles for Phoenix projects
type PhoenixProvider struct {
	providers.BaseProvider
}

// NewPhoenixProvider creates a new Phoenix provider
func NewPhoenixProvider() *PhoenixProvider {
	return &PhoenixProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "phoenix",
			ProviderLanguage:    "elixir",
			ProviderFramework:   "phoenix",
			ProviderTemplate:    "elixir/phoenix.tmpl",
			ProviderDescription: "Phoenix web framework for Elixir",
			ProviderURL:         "https://phoenixframework.org",
		},
	}
}

// Detect checks if the repository is a Phoenix project
func (p *PhoenixProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have mix.exs
	if !scan.FileTree.HasFile("mix.exs") {
		return 0, nil, nil
	}

	// Read mix.exs to check for Phoenix dependency
	data, err := scan.ReadFile("mix.exs")
	if err != nil {
		return 0, nil, nil
	}

	content := string(data)

	// Check for phoenix dependency
	if strings.Contains(content, ":phoenix") {
		score += 50
	} else {
		return 0, nil, nil // Not Phoenix
	}

	// Extract app name from mix.exs (pattern: app: :app_name)
	appNameRe := regexp.MustCompile(`app:\s*:(\w+)`)
	if matches := appNameRe.FindStringSubmatch(content); len(matches) > 1 {
		vars["appName"] = matches[1]
	}

	// Check for phoenix_html
	if strings.Contains(content, ":phoenix_html") {
		score += 10
		vars["hasPhoenixHTML"] = true
	}

	// Check for phoenix_live_view
	if strings.Contains(content, ":phoenix_live_view") {
		score += 10
		vars["hasLiveView"] = true
	}

	// Check for ecto (database)
	if strings.Contains(content, ":ecto") || strings.Contains(content, ":phoenix_ecto") {
		vars["hasEcto"] = true
	}

	// Extract Elixir version from mix.exs
	elixirVersionRe := regexp.MustCompile(`elixir:\s*"~>\s*(\d+\.\d+)`)
	if matches := elixirVersionRe.FindStringSubmatch(content); len(matches) > 1 {
		vars["elixirVersion"] = matches[1]
	}

	// Check for config directory
	if scan.FileTree.HasDir("config") {
		score += 5
	}

	// Check for lib directory
	if scan.FileTree.HasDir("lib") {
		score += 5
	}

	// Check for assets directory (Phoenix assets pipeline)
	if scan.FileTree.HasDir("assets") {
		score += 5
		vars["hasAssets"] = true
	}

	// Check for priv/static
	if scan.FileTree.HasDir("priv/static") {
		score += 5
	}

	// Check for .tool-versions (asdf)
	if scan.FileTree.HasFile(".tool-versions") {
		data, err := scan.ReadFile(".tool-versions")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "elixir ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						vars["elixirVersion"] = extractElixirVersion(parts[1])
					}
				}
				if strings.HasPrefix(line, "erlang ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						vars["erlangVersion"] = parts[1]
					}
				}
			}
		}
	}

	// Default versions
	if _, ok := vars["elixirVersion"]; !ok {
		vars["elixirVersion"] = "1.16"
	}
	if _, ok := vars["erlangVersion"]; !ok {
		vars["erlangVersion"] = "26"
	}

	// Default port
	vars["port"] = "4000"

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Elixir version
func (p *PhoenixProvider) DetectVersion(scan *scanner.ScanResult) string {
	// Check .tool-versions
	if scan.FileTree.HasFile(".tool-versions") {
		data, err := scan.ReadFile(".tool-versions")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "elixir ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						return extractElixirVersion(parts[1])
					}
				}
			}
		}
	}

	// Check mix.exs for elixir version requirement
	if scan.FileTree.HasFile("mix.exs") {
		data, err := scan.ReadFile("mix.exs")
		if err == nil {
			re := regexp.MustCompile(`elixir:\s*"~>\s*(\d+\.\d+)`)
			if matches := re.FindStringSubmatch(string(data)); len(matches) > 1 {
				return matches[1]
			}
		}
	}

	return "1.16"
}

// extractElixirVersion extracts version from various formats
func extractElixirVersion(version string) string {
	// Handle formats like "1.16.0-otp-26" or "1.16.0"
	parts := strings.Split(version, "-")
	if len(parts) >= 1 {
		versionParts := strings.Split(parts[0], ".")
		if len(versionParts) >= 2 {
			return versionParts[0] + "." + versionParts[1]
		}
	}
	return "1.16"
}
