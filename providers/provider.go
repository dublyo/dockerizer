// Package providers defines the provider interface and base types for stack detection.
// Copyright (c) 2026 Dublyo. All rights reserved.
// Licensed under the MIT License.
package providers

import (
	"context"

	"github.com/dublyo/dockerizer/internal/scanner"
)

// Provider detects and generates for a specific stack
type Provider interface {
	// Identity
	Name() string      // e.g., "nextjs"
	Language() string  // e.g., "nodejs"
	Framework() string // e.g., "nextjs"

	// Detection
	Detect(ctx context.Context, scan *scanner.ScanResult) (confidence int, variables map[string]interface{}, err error)
	DetectVersion(scan *scanner.ScanResult) string

	// Generation
	Template() string // Template file path

	// Metadata for display
	Description() string
	URL() string
}

// BaseProvider provides common functionality
type BaseProvider struct {
	ProviderName        string
	ProviderLanguage    string
	ProviderFramework   string
	ProviderTemplate    string
	ProviderDescription string
	ProviderURL         string
}

func (p *BaseProvider) Name() string        { return p.ProviderName }
func (p *BaseProvider) Language() string    { return p.ProviderLanguage }
func (p *BaseProvider) Framework() string   { return p.ProviderFramework }
func (p *BaseProvider) Template() string    { return p.ProviderTemplate }
func (p *BaseProvider) Description() string { return p.ProviderDescription }
func (p *BaseProvider) URL() string         { return p.ProviderURL }

// Rule defines a detection rule
type Rule struct {
	// Files that MUST exist
	RequiredFiles []string

	// Files where at least ONE must exist
	IndicatorFiles []string

	// Package dependencies to check
	Dependencies struct {
		Required []string
		Optional []string
	}

	// Content patterns
	Patterns []Pattern

	// Scoring weights
	Weights struct {
		RequiredFiles  int
		IndicatorFiles int
		Dependencies   int
		Patterns       int
	}
}

// Pattern defines a content pattern to match
type Pattern struct {
	File  string
	Regex string
}

// DefaultWeights returns standard scoring weights
func DefaultWeights() struct {
	RequiredFiles  int
	IndicatorFiles int
	Dependencies   int
	Patterns       int
} {
	return struct {
		RequiredFiles  int
		IndicatorFiles int
		Dependencies   int
		Patterns       int
	}{
		RequiredFiles:  25,
		IndicatorFiles: 25,
		Dependencies:   30,
		Patterns:       20,
	}
}
