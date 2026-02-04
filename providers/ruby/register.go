package ruby

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all Ruby providers with the registry
func RegisterAll(registry *detector.Registry) {
	// Register in order of specificity
	registry.Register(NewRailsProvider())
	// Future providers:
	// registry.Register(NewSinatraProvider())
	// registry.Register(NewHanamiProvider())
}
