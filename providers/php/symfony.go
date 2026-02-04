package php

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// SymfonyProvider detects and generates Dockerfiles for Symfony projects
type SymfonyProvider struct {
	providers.BaseProvider
}

// NewSymfonyProvider creates a new Symfony provider
func NewSymfonyProvider() *SymfonyProvider {
	return &SymfonyProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "symfony",
			ProviderLanguage:    "php",
			ProviderFramework:   "symfony",
			ProviderTemplate:    "php/symfony.tmpl",
			ProviderDescription: "Symfony PHP web framework",
			ProviderURL:         "https://symfony.com",
		},
	}
}

// Detect checks if the repository is a Symfony project
func (p *SymfonyProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
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

	// Check for symfony/framework-bundle in require
	require, ok := composer["require"].(map[string]interface{})
	if !ok {
		return 0, nil, nil
	}

	// Check for Symfony framework bundle (required)
	if _, hasSymfony := require["symfony/framework-bundle"]; hasSymfony {
		score += 50
	} else if _, hasSymfony := require["symfony/symfony"]; hasSymfony {
		score += 50
	} else {
		return 0, nil, nil // Not Symfony
	}

	// Check for symfony.lock
	if scan.FileTree.HasFile("symfony.lock") {
		score += 15
	}

	// Check for Symfony CLI config
	if scan.FileTree.HasFile(".symfony.local.yaml") || scan.FileTree.HasFile(".symfony/services.yaml") {
		score += 5
	}

	// Check for bin/console (Symfony CLI)
	if scan.FileTree.HasFile("bin/console") {
		score += 10
	}

	// Check for config/bundles.php
	if scan.FileTree.HasFile("config/bundles.php") {
		score += 5
	}

	// Check for src/Kernel.php
	if scan.FileTree.HasFile("src/Kernel.php") {
		score += 5
	}

	// Check for Symfony-specific directories
	if scan.FileTree.HasDir("config") {
		score += 5
	}
	if scan.FileTree.HasDir("templates") {
		vars["hasTwig"] = true
	}
	if scan.FileTree.HasDir("public") {
		score += 5
	}

	// Check for Doctrine (database)
	if _, hasDoctrine := require["doctrine/doctrine-bundle"]; hasDoctrine {
		vars["hasDoctrine"] = true
	}

	// Check for API Platform
	if _, hasAPI := require["api-platform/core"]; hasAPI {
		vars["hasAPIPlatform"] = true
	}

	// Detect PHP version from require
	vars["phpVersion"] = detectPhpVersion(scan, require)

	// Check for Encore (Webpack)
	if scan.FileTree.HasFile("webpack.config.js") {
		vars["hasEncore"] = true
	}

	// Check for asset-mapper
	if scan.FileTree.HasFile("importmap.php") {
		vars["hasAssetMapper"] = true
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
func (p *SymfonyProvider) DetectVersion(scan *scanner.ScanResult) string {
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

// detectPhpVersion is defined in laravel.go, but we include here for independence
func detectSymfonyPhpVersion(scan *scanner.ScanResult, require map[string]interface{}) string {
	// Check composer.json require.php
	if phpVersion, ok := require["php"].(string); ok {
		return parseSymfonyPhpVersion(phpVersion)
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

// parseSymfonyPhpVersion extracts PHP version from constraint
func parseSymfonyPhpVersion(constraint string) string {
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
