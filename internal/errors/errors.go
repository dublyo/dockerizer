// Package errors provides centralized error definitions for dockerizer.
package errors

import "errors"

// Detection errors
var (
	ErrNoProviderMatch = errors.New("no provider matched the repository")
	ErrLowConfidence   = errors.New("detection confidence below threshold")
	ErrEmptyRepository = errors.New("repository is empty or contains no recognizable files")
)

// AI errors
var (
	ErrAINotConfigured   = errors.New("AI required but no API key configured")
	ErrAIRequestFailed   = errors.New("AI provider request failed")
	ErrAIResponseInvalid = errors.New("AI response could not be parsed")
	ErrAIRateLimited     = errors.New("AI provider rate limit exceeded")
)

// Template errors
var (
	ErrTemplateNotFound = errors.New("template file not found")
	ErrTemplateInvalid  = errors.New("template contains syntax errors")
	ErrVariableMissing  = errors.New("required template variable not provided")
)

// Validation errors
var (
	ErrInvalidDockerfile = errors.New("generated Dockerfile is invalid")
	ErrMissingFROM       = errors.New("Dockerfile missing FROM instruction")
	ErrInvalidCompose    = errors.New("docker-compose.yml has invalid syntax")
)

// Config errors
var (
	ErrConfigInvalid  = errors.New("configuration file is invalid")
	ErrConfigNotFound = errors.New("configuration file not found")
)

// Scanner errors
var (
	ErrPathNotFound  = errors.New("specified path does not exist")
	ErrNotADirectory = errors.New("specified path is not a directory")
	ErrAccessDenied  = errors.New("access denied to path")
	ErrScanCancelled = errors.New("scan was cancelled")
)

// Generator errors
var (
	ErrOutputPathInvalid = errors.New("output path is invalid")
	ErrWriteFailed       = errors.New("failed to write output file")
)
