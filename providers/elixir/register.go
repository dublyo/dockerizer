package elixir

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all Elixir providers with the registry
func RegisterAll(registry *detector.Registry) {
	registry.Register(NewPhoenixProvider())
	// Future providers:
	// registry.Register(NewNerves Provider())
}
