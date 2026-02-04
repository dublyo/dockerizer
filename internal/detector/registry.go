package detector

import (
	"sync"

	"github.com/dublyo/dockerizer/providers"
)

// Registry holds all registered providers
type Registry struct {
	mu        sync.RWMutex
	providers map[string]providers.Provider
	ordered   []providers.Provider // Maintains registration order
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]providers.Provider),
		ordered:   make([]providers.Provider, 0),
	}
}

// Register adds a provider to the registry
func (r *Registry) Register(p providers.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[p.Name()]; exists {
		// Replace existing provider
		for i, existing := range r.ordered {
			if existing.Name() == p.Name() {
				r.ordered[i] = p
				break
			}
		}
	} else {
		r.ordered = append(r.ordered, p)
	}
	r.providers[p.Name()] = p
}

// Get returns a provider by name
func (r *Registry) Get(name string) providers.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[name]
}

// Providers returns all registered providers
func (r *Registry) Providers() []providers.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]providers.Provider, len(r.ordered))
	copy(result, r.ordered)
	return result
}

// ByLanguage returns providers for a specific language
func (r *Registry) ByLanguage(language string) []providers.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []providers.Provider
	for _, p := range r.ordered {
		if p.Language() == language {
			result = append(result, p)
		}
	}
	return result
}

// Languages returns all unique languages
func (r *Registry) Languages() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]struct{})
	var languages []string
	for _, p := range r.ordered {
		if _, ok := seen[p.Language()]; !ok {
			seen[p.Language()] = struct{}{}
			languages = append(languages, p.Language())
		}
	}
	return languages
}

// Count returns the number of registered providers
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}
