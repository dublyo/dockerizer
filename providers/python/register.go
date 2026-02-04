package python

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all Python providers with the registry
func RegisterAll(registry *detector.Registry) {
	// Register in order of specificity (more specific first)
	registry.Register(NewFastAPIProvider())
	registry.Register(NewDjangoProvider())
	registry.Register(NewFlaskProvider())
}
