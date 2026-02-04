// Package scanner provides repository scanning functionality.
package scanner

import (
	"os"
	"path/filepath"
)

// ScanResult contains all information extracted from a repository
type ScanResult struct {
	Path     string
	FileTree *FileTree
	Metadata *Metadata
	KeyFiles []KeyFile
	rootPath string // For ReadFile operations
}

// ReadFile reads a file relative to the repository root
func (s *ScanResult) ReadFile(path string) ([]byte, error) {
	fullPath := filepath.Join(s.rootPath, path)
	return os.ReadFile(fullPath)
}

// HasFile checks if a file exists in the scan result
func (s *ScanResult) HasFile(path string) bool {
	return s.FileTree.HasFile(path)
}

// HasDir checks if a directory exists in the scan result
func (s *ScanResult) HasDir(path string) bool {
	return s.FileTree.HasDir(path)
}

// FileTree represents the repository structure
type FileTree struct {
	Root     string
	Files    []string
	Dirs     []string
	MaxDepth int
	fileSet  map[string]struct{} // For fast lookup
	dirSet   map[string]struct{} // For fast lookup
}

// HasFile checks if a file exists in the tree
func (ft *FileTree) HasFile(path string) bool {
	if ft.fileSet == nil {
		ft.buildSets()
	}
	_, ok := ft.fileSet[path]
	return ok
}

// HasDir checks if a directory exists in the tree
func (ft *FileTree) HasDir(path string) bool {
	if ft.dirSet == nil {
		ft.buildSets()
	}
	_, ok := ft.dirSet[path]
	return ok
}

// buildSets builds the lookup maps for fast file/dir checking
func (ft *FileTree) buildSets() {
	ft.fileSet = make(map[string]struct{}, len(ft.Files))
	for _, f := range ft.Files {
		ft.fileSet[f] = struct{}{}
	}
	ft.dirSet = make(map[string]struct{}, len(ft.Dirs))
	for _, d := range ft.Dirs {
		ft.dirSet[d] = struct{}{}
	}
}

// FilesWithExtension returns files matching the given extension
func (ft *FileTree) FilesWithExtension(ext string) []string {
	var result []string
	for _, f := range ft.Files {
		if filepath.Ext(f) == ext {
			result = append(result, f)
		}
	}
	return result
}

// FilesMatching returns files matching the given pattern
func (ft *FileTree) FilesMatching(pattern string) []string {
	var result []string
	for _, f := range ft.Files {
		if matched, _ := filepath.Match(pattern, filepath.Base(f)); matched {
			result = append(result, f)
		}
	}
	return result
}

// Metadata contains parsed configuration files
type Metadata struct {
	PackageJSON  *PackageJSON  // package.json
	GoMod        *GoMod        // go.mod
	PyProject    *PyProject    // pyproject.toml
	Requirements []string      // requirements.txt lines
	Gemfile      *Gemfile      // Gemfile
	CargoToml    *CargoToml    // Cargo.toml
	ComposerJSON *ComposerJSON // composer.json
	PomXML       *PomXML       // pom.xml
	Csproj       *Csproj       // *.csproj
}

// PackageJSON represents a Node.js package.json file
type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Main            string            `json:"main"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         struct {
		Node string `json:"node"`
		NPM  string `json:"npm"`
	} `json:"engines"`
	PackageManager string `json:"packageManager"`
	Type           string `json:"type"` // "module" or "commonjs"
}

// HasDependency checks if a dependency exists (dev or regular)
func (p *PackageJSON) HasDependency(name string) bool {
	if p == nil {
		return false
	}
	if _, ok := p.Dependencies[name]; ok {
		return true
	}
	if _, ok := p.DevDependencies[name]; ok {
		return true
	}
	return false
}

// HasScript checks if a script exists
func (p *PackageJSON) HasScript(name string) bool {
	if p == nil {
		return false
	}
	_, ok := p.Scripts[name]
	return ok
}

// GoMod represents a Go go.mod file
type GoMod struct {
	Module  string
	Go      string   // Go version (e.g., "1.21")
	Require []string // Module dependencies
}

// PyProject represents a Python pyproject.toml file
type PyProject struct {
	Name          string
	Version       string
	PythonVersion string
	Dependencies  []string
	BuildSystem   string // poetry, setuptools, flit, etc.
}

// Gemfile represents a Ruby Gemfile
type Gemfile struct {
	RubyVersion string
	Gems        []string
	Source      string
}

// CargoToml represents a Rust Cargo.toml file
type CargoToml struct {
	Name         string
	Version      string
	Edition      string // 2018, 2021
	Dependencies []string
}

// ComposerJSON represents a PHP composer.json file
type ComposerJSON struct {
	Name     string            `json:"name"`
	Require  map[string]string `json:"require"`
	Autoload struct {
		PSR4 map[string]string `json:"psr-4"`
	} `json:"autoload"`
}

// PomXML represents a Java pom.xml file
type PomXML struct {
	GroupID      string
	ArtifactID   string
	Version      string
	JavaVersion  string
	Dependencies []string
}

// Csproj represents a .NET .csproj file
type Csproj struct {
	TargetFramework string
	OutputType      string
	IsWeb           bool
}

// KeyFile is a file that should be included in AI context
type KeyFile struct {
	Path    string
	Content string
	Size    int64
}
