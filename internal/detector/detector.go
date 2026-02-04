package detector

import (
	"context"
	"sort"

	"github.com/dublyo/dockerizer/internal/scanner"
)

// Detector detects the stack of a repository
type Detector interface {
	Detect(ctx context.Context, scan *scanner.ScanResult) (*DetectionResult, error)
}

// Option configures the detector
type Option func(*detector)

// detector implements Detector
type detector struct {
	registry      *Registry
	minConfidence int // Default 80, below this triggers AI
}

// New creates a new detector
func New(registry *Registry, opts ...Option) Detector {
	d := &detector{
		registry:      registry,
		minConfidence: 80,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// WithMinConfidence sets the minimum confidence threshold
func WithMinConfidence(confidence int) Option {
	return func(d *detector) {
		d.minConfidence = confidence
	}
}

// Detect runs detection against all registered providers
func (d *detector) Detect(ctx context.Context, scan *scanner.ScanResult) (*DetectionResult, error) {
	var candidates []Candidate

	// Run all providers
	for _, p := range d.registry.Providers() {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		score, vars, err := p.Detect(ctx, scan)
		if err != nil {
			// Log error but continue with other providers
			continue
		}

		if score > 0 {
			candidates = append(candidates, Candidate{
				Provider:   p.Name(),
				Confidence: score,
				Variables:  vars,
			})
		}
	}

	// Sort by confidence descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Confidence > candidates[j].Confidence
	})

	if len(candidates) == 0 {
		return &DetectionResult{
			Detected:   false,
			Candidates: candidates,
		}, nil
	}

	best := candidates[0]
	provider := d.registry.Get(best.Provider)

	return &DetectionResult{
		Detected:   true,
		Confidence: best.Confidence,
		Language:   provider.Language(),
		Framework:  provider.Framework(),
		Version:    provider.DetectVersion(scan),
		Provider:   best.Provider,
		Template:   provider.Template(),
		Variables:  best.Variables,
		Candidates: candidates,
	}, nil
}

// MinConfidence returns the minimum confidence threshold
func (d *detector) MinConfidence() int {
	return d.minConfidence
}
