package java

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all Java providers with the registry
func RegisterAll(registry *detector.Registry) {
	// Register in order of specificity
	registry.Register(NewQuarkusProvider())
	registry.Register(NewSpringBootProvider())
	// Future providers:
	// registry.Register(NewMicronautProvider())
	// registry.Register(NewJakartaEEProvider())
}
