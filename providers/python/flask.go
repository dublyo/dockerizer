package python

import (
	"context"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// FlaskProvider detects Flask projects
type FlaskProvider struct {
	providers.BaseProvider
}

// NewFlaskProvider creates a new Flask provider
func NewFlaskProvider() *FlaskProvider {
	return &FlaskProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "flask",
			ProviderLanguage:    "python",
			ProviderFramework:   "flask",
			ProviderTemplate:    "python/flask.tmpl",
			ProviderDescription: "Flask micro web framework",
			ProviderURL:         "https://flask.palletsprojects.com",
		},
	}
}

// Detect checks if the repository is a Flask project
func (p *FlaskProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Check requirements.txt for flask
	for _, req := range scan.Metadata.Requirements {
		if strings.HasPrefix(strings.ToLower(req), "flask") {
			score += 50
			break
		}
	}

	// Check for app.py or main.py with Flask import
	mainFiles := []string{"app.py", "main.py", "application.py", "wsgi.py", "src/app.py"}
	for _, mf := range mainFiles {
		if scan.FileTree.HasFile(mf) {
			data, err := scan.ReadFile(mf)
			if err == nil {
				content := string(data)
				if strings.Contains(content, "from flask import") || strings.Contains(content, "import flask") {
					score += 30
					vars["mainFile"] = mf
					break
				}
			}
		}
	}

	// Check pyproject.toml
	if scan.Metadata.PyProject != nil {
		for _, dep := range scan.Metadata.PyProject.Dependencies {
			if strings.Contains(strings.ToLower(dep), "flask") {
				score += 30
				break
			}
		}
	}

	// Check for Flask extensions (Flask-* packages)
	flaskExtensions := []string{"flask-cors", "flask-sqlalchemy", "flask-migrate", "flask-login", "flask-wtf", "flask-restful", "flask-jwt-extended"}
	for _, ext := range flaskExtensions {
		for _, req := range scan.Metadata.Requirements {
			if strings.HasPrefix(strings.ToLower(req), ext) {
				score += 5
				break
			}
		}
	}

	// Check for WSGI server (gunicorn/waitress)
	for _, req := range scan.Metadata.Requirements {
		reqLower := strings.ToLower(req)
		if strings.HasPrefix(reqLower, "gunicorn") || strings.HasPrefix(reqLower, "waitress") {
			score += 10
			break
		}
	}

	// Check for templates directory (Flask convention)
	if scan.FileTree.HasDir("templates") {
		score += 10
		vars["hasTemplates"] = true
	}

	// Check for static directory
	if scan.FileTree.HasDir("static") {
		score += 5
		vars["hasStatic"] = true
	}

	if score == 0 {
		return 0, nil, nil
	}

	// Set defaults
	vars["pythonVersion"] = p.DetectVersion(scan)
	vars["packageManager"] = detectPythonPackageManager(scan)
	vars["wsgiServer"] = detectWSGIServer(scan)

	// Set mainFile for FLASK_APP and module name for gunicorn
	if vars["mainFile"] == nil {
		vars["mainFile"] = "app.py"
	}
	// Strip .py extension for gunicorn module name
	mainFile := vars["mainFile"].(string)
	moduleName := strings.TrimSuffix(mainFile, ".py")
	// Handle src/app.py -> src.app
	moduleName = strings.ReplaceAll(moduleName, "/", ".")
	vars["moduleName"] = moduleName

	vars["port"] = "5000"

	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the Python version
func (p *FlaskProvider) DetectVersion(scan *scanner.ScanResult) string {
	return detectPythonVersion(scan)
}
