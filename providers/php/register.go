package php

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all PHP providers with the registry
func RegisterAll(registry *detector.Registry) {
	// Register in order of specificity
	registry.Register(NewLaravelProvider())
	registry.Register(NewSymfonyProvider())
}
