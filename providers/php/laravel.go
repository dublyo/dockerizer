package php

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// LaravelProvider detects and generates Dockerfiles for Laravel projects
type LaravelProvider struct {
	providers.BaseProvider
}

// NewLaravelProvider creates a new Laravel provider
func NewLaravelProvider() *LaravelProvider {
	return &LaravelProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "laravel",
			ProviderLanguage:    "php",
			ProviderFramework:   "laravel",
			ProviderTemplate:    "php/laravel.tmpl",
			ProviderDescription: "Laravel PHP web framework",
			ProviderURL:         "https://laravel.com",
		},
	}
}

// Detect checks if the repository is a Laravel project
func (p *LaravelProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Must have composer.json
	if !scan.FileTree.HasFile("composer.json") {
		return 0, nil, nil
	}

	// Read and parse composer.json
	data, err := scan.ReadFile("composer.json")
	if err != nil {
		return 0, nil, nil
	}

	var composer map[string]interface{}
	if err := json.Unmarshal(data, &composer); err != nil {
		return 0, nil, nil
	}

	// Check for laravel/framework in require
	require, ok := composer["require"].(map[string]interface{})
	if !ok {
		return 0, nil, nil
	}

	if _, hasLaravel := require["laravel/framework"]; hasLaravel {
		score += 50
	} else {
		return 0, nil, nil // Not Laravel
	}

	// Check for artisan file
	if scan.FileTree.HasFile("artisan") {
		score += 20
	}

	// Check for Laravel-specific directories
	if scan.FileTree.HasDir("app/Http") {
		score += 10
	}
	if scan.FileTree.HasDir("resources/views") {
		score += 5
	}
	if scan.FileTree.HasDir("routes") {
		score += 5
	}
	if scan.FileTree.HasDir("database/migrations") {
		score += 5
	}

	// Check for .env.example
	if scan.FileTree.HasFile(".env.example") {
		score += 5
	}

	// Check for composer.lock
	vars["hasLockFile"] = scan.FileTree.HasFile("composer.lock")

	// Detect PHP version from require
	vars["phpVersion"] = detectPhpVersion(scan, require)

	// Check for database driver
	if _, hasPdo := require["ext-pdo"]; hasPdo {
		vars["database"] = "mysql" // Default
	}

	// Check for queue driver (Redis)
	if _, hasRedis := require["predis/predis"]; hasRedis {
		vars["hasRedis"] = true
	}
	if _, hasPhpRedis := require["ext-redis"]; hasPhpRedis {
		vars["hasRedis"] = true
	}

	// Check for Laravel Octane
	if _, hasOctane := require["laravel/octane"]; hasOctane {
		vars["hasOctane"] = true
	}

	// Check for Vite or Mix
	if scan.FileTree.HasFile("vite.config.js") || scan.FileTree.HasFile("vite.config.ts") {
		vars["hasVite"] = true
	} else if scan.FileTree.HasFile("webpack.mix.js") {
		vars["hasMix"] = true
	}

	// Default port
	vars["port"] = "8000"

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the PHP version
func (p *LaravelProvider) DetectVersion(scan *scanner.ScanResult) string {
	// Read composer.json to get PHP version
	data, err := scan.ReadFile("composer.json")
	if err != nil {
		return "8.3"
	}

	var composer map[string]interface{}
	if err := json.Unmarshal(data, &composer); err != nil {
		return "8.3"
	}

	require, ok := composer["require"].(map[string]interface{})
	if !ok {
		return "8.3"
	}

	return detectPhpVersion(scan, require)
}

// detectPhpVersion extracts PHP version from composer.json or files
func detectPhpVersion(scan *scanner.ScanResult, require map[string]interface{}) string {
	// Check composer.json require.php
	if phpVersion, ok := require["php"].(string); ok {
		return parsePhpVersion(phpVersion)
	}

	// Check .php-version file
	if scan.FileTree.HasFile(".php-version") {
		data, err := scan.ReadFile(".php-version")
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	return "8.3"
}

// parsePhpVersion extracts PHP version from constraint
func parsePhpVersion(constraint string) string {
	// Handle common version constraints
	constraint = strings.TrimPrefix(constraint, "^")
	constraint = strings.TrimPrefix(constraint, ">=")
	constraint = strings.TrimPrefix(constraint, "~")

	// Extract major.minor
	parts := strings.Split(constraint, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	if len(parts) == 1 {
		return parts[0]
	}

	return "8.3"
}
