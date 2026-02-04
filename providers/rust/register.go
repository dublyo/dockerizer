package rust

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all Rust providers with the registry
func RegisterAll(registry *detector.Registry) {
	registry.Register(NewActixProvider())
	registry.Register(NewAxumProvider())
}
