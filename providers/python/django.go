// Package python provides detection for Python frameworks.
package python

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// DjangoProvider detects Django projects
type DjangoProvider struct {
	providers.BaseProvider
}

// NewDjangoProvider creates a new Django provider
func NewDjangoProvider() *DjangoProvider {
	return &DjangoProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "django",
			ProviderLanguage:    "python",
			ProviderFramework:   "django",
			ProviderTemplate:    "python/django.tmpl",
			ProviderDescription: "Django Web Framework",
			ProviderURL:         "https://www.djangoproject.com",
		},
	}
}

// Detect checks if the repository is a Django project
func (p *DjangoProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Check for manage.py (Django signature file)
	if scan.FileTree.HasFile("manage.py") {
		score += 40
	}

	// Check requirements.txt for django
	if hasDjangoInRequirements(scan) {
		score += 30
	}

	// Check pyproject.toml for django
	if scan.Metadata.PyProject != nil {
		for _, dep := range scan.Metadata.PyProject.Dependencies {
			if strings.Contains(strings.ToLower(dep), "django") {
				score += 30
				break
			}
		}
	}

	// Check for settings.py or <project>/settings.py and extract project name
	settingsFiles := scan.FileTree.FilesMatching("settings.py")
	if len(settingsFiles) > 0 {
		score += 20
		// Extract project name from settings.py path (e.g., "myproject/settings.py" -> "myproject")
		// Convert filepath to Python module path (e.g., "src/myproj" -> "src.myproj")
		for _, sf := range settingsFiles {
			dir := filepath.Dir(sf)
			if dir != "." && dir != "" {
				vars["projectName"] = strings.ReplaceAll(dir, "/", ".")
				break
			}
		}
	}

	// Check for wsgi.py or asgi.py and extract project name if not found
	wsgiFiles := scan.FileTree.FilesMatching("wsgi.py")
	asgiFiles := scan.FileTree.FilesMatching("asgi.py")
	if len(wsgiFiles) > 0 || len(asgiFiles) > 0 {
		score += 10
		// If projectName not set, try to get it from wsgi/asgi path
		if _, ok := vars["projectName"]; !ok {
			allFiles := append(wsgiFiles, asgiFiles...)
			for _, f := range allFiles {
				dir := filepath.Dir(f)
				if dir != "." && dir != "" {
					vars["projectName"] = strings.ReplaceAll(dir, "/", ".")
					break
				}
			}
		}
	}

	if score == 0 {
		return 0, nil, nil
	}

	// Default project name if not detected
	if _, ok := vars["projectName"]; !ok {
		vars["projectName"] = "config"
	}

	// Detect Python version
	vars["pythonVersion"] = p.DetectVersion(scan)

	// Detect package manager
	vars["packageManager"] = detectPythonPackageManager(scan)

	// Check for gunicorn/uvicorn
	vars["wsgiServer"] = detectWSGIServer(scan)

	// Check for static files
	if scan.FileTree.HasDir("static") || scan.FileTree.HasDir("staticfiles") {
		vars["hasStatic"] = true
	}

	// Default port
	vars["port"] = "8000"

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Python version
func (p *DjangoProvider) DetectVersion(scan *scanner.ScanResult) string {
	return detectPythonVersion(scan)
}

func hasDjangoInRequirements(scan *scanner.ScanResult) bool {
	for _, req := range scan.Metadata.Requirements {
		if strings.HasPrefix(strings.ToLower(req), "django") {
			return true
		}
	}
	return false
}

func detectPythonPackageManager(scan *scanner.ScanResult) string {
	if scan.FileTree.HasFile("poetry.lock") {
		return "poetry"
	}
	if scan.FileTree.HasFile("Pipfile.lock") || scan.FileTree.HasFile("Pipfile") {
		return "pipenv"
	}
	if scan.FileTree.HasFile("uv.lock") {
		return "uv"
	}
	if scan.Metadata.PyProject != nil && scan.Metadata.PyProject.BuildSystem == "poetry" {
		return "poetry"
	}
	return "pip"
}

func detectWSGIServer(scan *scanner.ScanResult) string {
	for _, req := range scan.Metadata.Requirements {
		reqLower := strings.ToLower(req)
		if strings.HasPrefix(reqLower, "gunicorn") {
			return "gunicorn"
		}
		if strings.HasPrefix(reqLower, "uvicorn") {
			return "uvicorn"
		}
	}
	return "gunicorn" // Default
}

func detectPythonVersion(scan *scanner.ScanResult) string {
	// Check pyproject.toml
	if scan.Metadata.PyProject != nil && scan.Metadata.PyProject.PythonVersion != "" {
		re := regexp.MustCompile(`(\d+\.\d+)`)
		matches := re.FindString(scan.Metadata.PyProject.PythonVersion)
		if matches != "" {
			return matches
		}
	}

	// Check .python-version
	if scan.FileTree.HasFile(".python-version") {
		data, err := scan.ReadFile(".python-version")
		if err == nil {
			version := strings.TrimSpace(string(data))
			re := regexp.MustCompile(`(\d+\.\d+)`)
			matches := re.FindString(version)
			if matches != "" {
				return matches
			}
		}
	}

	// Check runtime.txt (Heroku style)
	if scan.FileTree.HasFile("runtime.txt") {
		data, err := scan.ReadFile("runtime.txt")
		if err == nil {
			content := strings.TrimSpace(string(data))
			re := regexp.MustCompile(`python-(\d+\.\d+)`)
			matches := re.FindStringSubmatch(content)
			if len(matches) > 1 {
				return matches[1]
			}
		}
	}

	// Default to Python 3.12
	return "3.12"
}
