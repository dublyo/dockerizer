package python

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// FastAPIProvider detects FastAPI projects
type FastAPIProvider struct {
	providers.BaseProvider
}

// NewFastAPIProvider creates a new FastAPI provider
func NewFastAPIProvider() *FastAPIProvider {
	return &FastAPIProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "fastapi",
			ProviderLanguage:    "python",
			ProviderFramework:   "fastapi",
			ProviderTemplate:    "python/fastapi.tmpl",
			ProviderDescription: "FastAPI modern Python web framework",
			ProviderURL:         "https://fastapi.tiangolo.com",
		},
	}
}

// Detect checks if the repository is a FastAPI project
func (p *FastAPIProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Check requirements.txt for fastapi
	for _, req := range scan.Metadata.Requirements {
		reqLower := strings.ToLower(req)
		if strings.HasPrefix(reqLower, "fastapi") {
			score += 50
			break
		}
	}

	// Check for main.py or app.py with FastAPI import
	mainFiles := []string{"main.py", "app.py", "src/main.py", "app/main.py"}
	for _, mf := range mainFiles {
		if scan.FileTree.HasFile(mf) {
			data, err := scan.ReadFile(mf)
			if err == nil && strings.Contains(string(data), "FastAPI") {
				score += 30
				vars["mainFile"] = mf
				break
			}
		}
	}

	// Check for uvicorn in requirements (common with FastAPI)
	for _, req := range scan.Metadata.Requirements {
		if strings.HasPrefix(strings.ToLower(req), "uvicorn") {
			score += 10
			vars["wsgiServer"] = "uvicorn"
			break
		}
	}

	// Check pyproject.toml
	if scan.Metadata.PyProject != nil {
		for _, dep := range scan.Metadata.PyProject.Dependencies {
			if strings.Contains(strings.ToLower(dep), "fastapi") {
				score += 30
				break
			}
		}
	}

	if score == 0 {
		return 0, nil, nil
	}

	// Set defaults
	vars["pythonVersion"] = p.DetectVersion(scan)
	vars["packageManager"] = detectPythonPackageManager(scan)
	if vars["wsgiServer"] == nil {
		vars["wsgiServer"] = "uvicorn"
	}
	if vars["mainFile"] == nil {
		vars["mainFile"] = "main.py"
	}
	vars["port"] = "8000"

	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Python version
func (p *FastAPIProvider) DetectVersion(scan *scanner.ScanResult) string {
	return detectPythonVersion(scan)
}
