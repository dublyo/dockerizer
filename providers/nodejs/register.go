package nodejs

import (
	"github.com/dublyo/dockerizer/internal/detector"
)

// RegisterAll registers all Node.js providers with the registry
func RegisterAll(registry *detector.Registry) {
	// Register in order of specificity (more specific first)
	registry.Register(NewNextJSProvider())
	registry.Register(NewNuxtProvider())
	registry.Register(NewNestJSProvider())
	registry.Register(NewRemixProvider())
	registry.Register(NewAstroProvider())
	registry.Register(NewSvelteKitProvider())
	registry.Register(NewHonoProvider())
	registry.Register(NewKoaProvider())
	registry.Register(NewExpressProvider()) // Express last as it's most generic
}
