package ruby

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// RailsProvider detects and generates Dockerfiles for Ruby on Rails projects
type RailsProvider struct {
	providers.BaseProvider
}

// NewRailsProvider creates a new Rails provider
func NewRailsProvider() *RailsProvider {
	return &RailsProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "rails",
			ProviderLanguage:    "ruby",
			ProviderFramework:   "rails",
			ProviderTemplate:    "ruby/rails.tmpl",
			ProviderDescription: "Ruby on Rails web framework",
			ProviderURL:         "https://rubyonrails.org",
		},
	}
}

// Detect checks if the repository is a Rails project
func (p *RailsProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have Gemfile
	if !scan.FileTree.HasFile("Gemfile") {
		return 0, nil, nil
	}

	// Check Gemfile for rails gem
	data, err := scan.ReadFile("Gemfile")
	if err != nil {
		return 0, nil, nil
	}

	gemfileContent := string(data)
	if !strings.Contains(gemfileContent, "rails") {
		return 0, nil, nil
	}
	score += 50

	// Check for Rails-specific directories
	if scan.FileTree.HasDir("app") {
		score += 10
	}
	if scan.FileTree.HasDir("config") {
		score += 10
	}
	if scan.FileTree.HasDir("db") {
		score += 10
	}

	// Check for config/routes.rb
	if scan.FileTree.HasFile("config/routes.rb") {
		score += 10
	}

	// Check for config/application.rb
	if scan.FileTree.HasFile("config/application.rb") {
		score += 5
	}

	// Check for bin/rails
	if scan.FileTree.HasFile("bin/rails") {
		score += 5
	}

	// Detect Ruby version
	vars["rubyVersion"] = p.DetectVersion(scan)

	// Check for database type
	if strings.Contains(gemfileContent, "pg") || strings.Contains(gemfileContent, "postgresql") {
		vars["database"] = "postgresql"
	} else if strings.Contains(gemfileContent, "mysql2") {
		vars["database"] = "mysql"
	} else if strings.Contains(gemfileContent, "sqlite") {
		vars["database"] = "sqlite"
	}

	// Check for asset pipeline
	if strings.Contains(gemfileContent, "sprockets") || scan.FileTree.HasDir("app/assets") {
		vars["hasAssets"] = true
	}

	// Check for webpacker/jsbundling
	if strings.Contains(gemfileContent, "webpacker") {
		vars["webpacker"] = true
	} else if strings.Contains(gemfileContent, "jsbundling-rails") {
		vars["jsbundling"] = true
	}

	// Check for API mode
	if scan.FileTree.HasFile("config/application.rb") {
		appData, _ := scan.ReadFile("config/application.rb")
		if strings.Contains(string(appData), "config.api_only") {
			vars["apiOnly"] = true
		}
	}

	// Default port
	vars["port"] = "3000"

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Ruby version
func (p *RailsProvider) DetectVersion(scan *scanner.ScanResult) string {
	// Check .ruby-version
	if scan.FileTree.HasFile(".ruby-version") {
		data, err := scan.ReadFile(".ruby-version")
		if err == nil {
			version := strings.TrimSpace(string(data))
			// Remove ruby- prefix if present
			version = strings.TrimPrefix(version, "ruby-")
			if version != "" {
				return version
			}
		}
	}

	// Check Gemfile for ruby version
	if scan.FileTree.HasFile("Gemfile") {
		data, err := scan.ReadFile("Gemfile")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "ruby") {
					// Parse ruby "3.2.0" or ruby '3.2.0'
					parts := strings.Split(line, "\"")
					if len(parts) >= 2 {
						return parts[1]
					}
					parts = strings.Split(line, "'")
					if len(parts) >= 2 {
						return parts[1]
					}
				}
			}
		}
	}

	return "3.3"
}
