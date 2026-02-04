// Package detector provides stack detection functionality.
package detector

// DetectionResult contains the detection outcome
type DetectionResult struct {
	Detected   bool
	Confidence int                    // 0-100
	Language   string                 // nodejs, python, go, etc.
	Framework  string                 // nextjs, django, gin, etc.
	Version    string                 // Node 20, Python 3.11, etc.
	Provider   string                 // Provider that matched
	Template   string                 // Template to use
	Variables  map[string]interface{} // Template variables

	// All candidates with scores (for debugging)
	Candidates []Candidate
}

// Candidate is a potential match
type Candidate struct {
	Provider   string
	Confidence int
	Variables  map[string]interface{} // Template variables extracted during detection
	Reason     string
}

// NeedsAI returns true if the detection confidence is below the threshold
func (r *DetectionResult) NeedsAI(threshold int) bool {
	return !r.Detected || r.Confidence < threshold
}

// BestCandidate returns the highest scoring candidate, if any
func (r *DetectionResult) BestCandidate() *Candidate {
	if len(r.Candidates) == 0 {
		return nil
	}
	return &r.Candidates[0]
}

// TopCandidates returns the top N candidates
func (r *DetectionResult) TopCandidates(n int) []Candidate {
	if len(r.Candidates) <= n {
		return r.Candidates
	}
	return r.Candidates[:n]
}
