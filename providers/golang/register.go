package golang

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all Go providers with the registry
func RegisterAll(registry *detector.Registry) {
	// Register in order of specificity (more specific first)
	registry.Register(NewGinProvider())
	registry.Register(NewEchoProvider())
	registry.Register(NewFiberProvider())
	registry.Register(NewStandardProvider()) // Fallback for any Go HTTP app
}
