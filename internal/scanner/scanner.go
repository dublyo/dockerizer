package scanner

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dublyo/dockerizer/internal/errors"
)

// Scanner scans repositories
type Scanner interface {
	Scan(ctx context.Context, path string) (*ScanResult, error)
}

// Option configures the scanner
type Option func(*scanner)

// scanner implements Scanner
type scanner struct {
	maxFileSize        int64
	maxFiles           int
	ignoreHidden       bool
	ignorePaths        []string
	allowedHiddenFiles map[string]struct{} // Important hidden files to always include
}

// New creates a new scanner
func New(opts ...Option) Scanner {
	s := &scanner{
		maxFileSize:  1024 * 1024, // 1MB
		maxFiles:     10000,
		ignoreHidden: true,
		ignorePaths: []string{
			"node_modules",
			".git",
			"vendor",
			"__pycache__",
			".venv",
			"venv",
			"dist",
			"build",
			".next",
			".nuxt",
			"target",
		},
		// Important hidden files for version detection and configuration
		allowedHiddenFiles: map[string]struct{}{
			".nvmrc":           {},
			".node-version":    {},
			".python-version":  {},
			".ruby-version":    {},
			".go-version":      {},
			".java-version":    {},
			".sdkmanrc":        {},
			".tool-versions":   {},
			".mise.toml":       {},
			".rtx.toml":        {},
			".env":             {},
			".env.example":     {},
			".env.local":       {},
			".dockerizer.yml":  {},
			".dockerizer.yaml": {},
			".editorconfig":    {},
			".gitignore":       {},
			".dockerignore":    {},
			".babelrc":         {},
			".eslintrc":        {},
			".prettierrc":      {},
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithMaxFileSize sets the maximum file size to read
func WithMaxFileSize(size int64) Option {
	return func(s *scanner) {
		s.maxFileSize = size
	}
}

// WithMaxFiles sets the maximum number of files to scan
func WithMaxFiles(count int) Option {
	return func(s *scanner) {
		s.maxFiles = count
	}
}

// WithIgnoreHidden sets whether to ignore hidden files
func WithIgnoreHidden(ignore bool) Option {
	return func(s *scanner) {
		s.ignoreHidden = ignore
	}
}

// Scan implements Scanner with context cancellation support
func (s *scanner) Scan(ctx context.Context, path string) (*ScanResult, error) {
	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Verify path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.ErrPathNotFound
		}
		return nil, errors.ErrAccessDenied
	}

	if !info.IsDir() {
		return nil, errors.ErrNotADirectory
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{
		Path:     absPath,
		rootPath: absPath,
	}

	// Scan file tree with periodic cancellation checks
	tree, err := s.scanFileTree(ctx, absPath)
	if err != nil {
		return nil, err
	}
	result.FileTree = tree

	// Extract metadata
	metadata, err := s.extractMetadata(ctx, absPath, tree)
	if err != nil {
		return nil, err
	}
	result.Metadata = metadata

	// Collect key files for AI context
	keyFiles, err := s.collectKeyFiles(ctx, absPath, tree)
	if err != nil {
		return nil, err
	}
	result.KeyFiles = keyFiles

	return result, nil
}

// scanFileTree builds the file tree structure
func (s *scanner) scanFileTree(ctx context.Context, root string) (*FileTree, error) {
	tree := &FileTree{
		Root:    root,
		Files:   make([]string, 0, 1000),
		Dirs:    make([]string, 0, 100),
		fileSet: make(map[string]struct{}, 1000),
		dirSet:  make(map[string]struct{}, 100),
	}

	fileCount := 0
	maxDepth := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Check for cancellation periodically
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Get relative path
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		// Skip root
		if relPath == "." {
			return nil
		}

		// Calculate depth
		depth := strings.Count(relPath, string(filepath.Separator))
		if depth > maxDepth {
			maxDepth = depth
		}

		// Check if should ignore
		baseName := filepath.Base(relPath)
		if s.ignoreHidden && strings.HasPrefix(baseName, ".") && baseName != "." {
			// Check if this is an allowed hidden file
			if _, allowed := s.allowedHiddenFiles[baseName]; !allowed {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		for _, ignorePath := range s.ignorePaths {
			if baseName == ignorePath || strings.HasPrefix(relPath, ignorePath+string(filepath.Separator)) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if d.IsDir() {
			tree.Dirs = append(tree.Dirs, relPath)
			tree.dirSet[relPath] = struct{}{}
		} else {
			if fileCount >= s.maxFiles {
				return nil // Silently skip if we've hit the limit
			}
			tree.Files = append(tree.Files, relPath)
			tree.fileSet[relPath] = struct{}{}
			fileCount++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	tree.MaxDepth = maxDepth
	return tree, nil
}

// extractMetadata parses configuration files
func (s *scanner) extractMetadata(ctx context.Context, root string, tree *FileTree) (*Metadata, error) {
	metadata := &Metadata{}

	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Parse package.json
	if tree.HasFile("package.json") {
		data, err := os.ReadFile(filepath.Join(root, "package.json"))
		if err == nil {
			var pkg PackageJSON
			if json.Unmarshal(data, &pkg) == nil {
				metadata.PackageJSON = &pkg
			}
		}
	}

	// Parse go.mod
	if tree.HasFile("go.mod") {
		data, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err == nil {
			metadata.GoMod = parseGoMod(string(data))
		}
	}

	// Parse requirements.txt
	if tree.HasFile("requirements.txt") {
		data, err := os.ReadFile(filepath.Join(root, "requirements.txt"))
		if err == nil {
			metadata.Requirements = parseRequirements(string(data))
		}
	}

	// Parse pyproject.toml
	if tree.HasFile("pyproject.toml") {
		data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
		if err == nil {
			metadata.PyProject = parsePyProject(string(data))
		}
	}

	// Parse Cargo.toml
	if tree.HasFile("Cargo.toml") {
		data, err := os.ReadFile(filepath.Join(root, "Cargo.toml"))
		if err == nil {
			metadata.CargoToml = parseCargoToml(string(data))
		}
	}

	// Parse composer.json
	if tree.HasFile("composer.json") {
		data, err := os.ReadFile(filepath.Join(root, "composer.json"))
		if err == nil {
			var composer ComposerJSON
			if json.Unmarshal(data, &composer) == nil {
				metadata.ComposerJSON = &composer
			}
		}
	}

	return metadata, nil
}

// collectKeyFiles gathers important files for AI context
func (s *scanner) collectKeyFiles(ctx context.Context, root string, tree *FileTree) ([]KeyFile, error) {
	keyFilePatterns := []string{
		"package.json",
		"go.mod",
		"requirements.txt",
		"pyproject.toml",
		"Cargo.toml",
		"composer.json",
		"Gemfile",
		"pom.xml",
		"build.gradle",
		"build.gradle.kts",
		"mix.exs",
		"Dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		".dockerizer.yml",
		".dockerizer.yaml",
		".nvmrc",
		".node-version",
		".python-version",
		".ruby-version",
		".java-version",
		".sdkmanrc",
		".tool-versions",
		".mise.toml",
		"Procfile",
	}

	var keyFiles []KeyFile
	for _, pattern := range keyFilePatterns {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if tree.HasFile(pattern) {
			fullPath := filepath.Join(root, pattern)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			if info.Size() > s.maxFileSize {
				continue
			}

			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}

			keyFiles = append(keyFiles, KeyFile{
				Path:    pattern,
				Content: string(data),
				Size:    info.Size(),
			})
		}
	}

	return keyFiles, nil
}

// parseGoMod parses a go.mod file
func parseGoMod(content string) *GoMod {
	gomod := &GoMod{
		Require: make([]string, 0),
	}
	lines := strings.Split(content, "\n")
	inRequire := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "module ") {
			gomod.Module = strings.TrimPrefix(line, "module ")
		} else if strings.HasPrefix(line, "go ") {
			gomod.Go = strings.TrimPrefix(line, "go ")
		} else if strings.HasPrefix(line, "require (") {
			inRequire = true
		} else if line == ")" && inRequire {
			inRequire = false
		} else if inRequire && line != "" && !strings.HasPrefix(line, "//") {
			// Parse dependency line like: github.com/gin-gonic/gin v1.9.1
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				gomod.Require = append(gomod.Require, parts[0])
			}
		} else if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			// Single line require: require github.com/gin-gonic/gin v1.9.1
			rest := strings.TrimPrefix(line, "require ")
			parts := strings.Fields(rest)
			if len(parts) >= 1 {
				gomod.Require = append(gomod.Require, parts[0])
			}
		}
	}

	return gomod
}

// parseRequirements parses a requirements.txt file
func parseRequirements(content string) []string {
	var reqs []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Extract package name (before version specifier)
		parts := strings.FieldsFunc(line, func(r rune) bool {
			return r == '=' || r == '>' || r == '<' || r == '[' || r == ';'
		})
		if len(parts) > 0 {
			reqs = append(reqs, strings.TrimSpace(parts[0]))
		}
	}

	return reqs
}

// parsePyProject parses a pyproject.toml file (simplified)
func parsePyProject(content string) *PyProject {
	pyproj := &PyProject{}
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name = ") {
			pyproj.Name = strings.Trim(strings.TrimPrefix(line, "name = "), "\"")
		} else if strings.HasPrefix(line, "requires-python = ") {
			pyproj.PythonVersion = strings.Trim(strings.TrimPrefix(line, "requires-python = "), "\"")
		}
	}

	// Detect build system
	if strings.Contains(content, "[tool.poetry]") {
		pyproj.BuildSystem = "poetry"
	} else if strings.Contains(content, "[build-system]") {
		if strings.Contains(content, "flit") {
			pyproj.BuildSystem = "flit"
		} else if strings.Contains(content, "hatchling") {
			pyproj.BuildSystem = "hatch"
		} else {
			pyproj.BuildSystem = "setuptools"
		}
	}

	return pyproj
}

// parseCargoToml parses a Cargo.toml file (simplified)
func parseCargoToml(content string) *CargoToml {
	cargo := &CargoToml{
		Dependencies: make([]string, 0),
	}
	lines := strings.Split(content, "\n")
	inDependencies := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "[package]") {
			inDependencies = false
		} else if strings.HasPrefix(line, "[dependencies]") {
			inDependencies = true
		} else if strings.HasPrefix(line, "[") {
			inDependencies = false
		}

		if strings.HasPrefix(line, "name = ") && !inDependencies {
			cargo.Name = strings.Trim(strings.TrimPrefix(line, "name = "), "\"")
		} else if strings.HasPrefix(line, "version = ") && !inDependencies {
			cargo.Version = strings.Trim(strings.TrimPrefix(line, "version = "), "\"")
		} else if strings.HasPrefix(line, "edition = ") {
			cargo.Edition = strings.Trim(strings.TrimPrefix(line, "edition = "), "\"")
		} else if inDependencies && strings.Contains(line, "=") && !strings.HasPrefix(line, "#") {
			// Parse dependency line like: actix-web = "4" or serde = { version = "1", ... }
			parts := strings.SplitN(line, "=", 2)
			if len(parts) >= 1 {
				depName := strings.TrimSpace(parts[0])
				if depName != "" {
					cargo.Dependencies = append(cargo.Dependencies, depName)
				}
			}
		}
	}

	return cargo
}
