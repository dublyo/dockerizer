package dotnet

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all .NET providers with the registry
func RegisterAll(registry *detector.Registry) {
	registry.Register(NewAspNetProvider())
	// Future providers:
	// registry.Register(NewBlazorProvider())
	// registry.Register(NewMauiProvider())
}
