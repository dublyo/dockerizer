package dotnet

import (
	"context"
	"encoding/xml"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// AspNetProvider detects and generates Dockerfiles for ASP.NET Core projects
type AspNetProvider struct {
	providers.BaseProvider
}

// NewAspNetProvider creates a new ASP.NET Core provider
func NewAspNetProvider() *AspNetProvider {
	return &AspNetProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "aspnet",
			ProviderLanguage:    "dotnet",
			ProviderFramework:   "aspnet",
			ProviderTemplate:    "dotnet/aspnet.tmpl",
			ProviderDescription: "ASP.NET Core web framework",
			ProviderURL:         "https://dotnet.microsoft.com/apps/aspnet",
		},
	}
}

// CsprojFile represents a .NET project file structure
type CsprojFile struct {
	XMLName       xml.Name `xml:"Project"`
	Sdk           string   `xml:"Sdk,attr"`
	PropertyGroup struct {
		TargetFramework string `xml:"TargetFramework"`
		Nullable        string `xml:"Nullable"`
		ImplicitUsings  string `xml:"ImplicitUsings"`
	} `xml:"PropertyGroup"`
	ItemGroup struct {
		PackageReferences []struct {
			Include string `xml:"Include,attr"`
			Version string `xml:"Version,attr"`
		} `xml:"PackageReference"`
	} `xml:"ItemGroup"`
}

// Detect checks if the repository is an ASP.NET Core project
func (p *AspNetProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Find .csproj files
	csprojFiles := scan.FileTree.FilesWithExtension(".csproj")
	if len(csprojFiles) == 0 {
		return 0, nil, nil
	}

	// Check for ASP.NET indicators
	for _, csprojFile := range csprojFiles {
		data, err := scan.ReadFile(csprojFile)
		if err != nil {
			continue
		}

		content := string(data)

		// Check for Web SDK
		if strings.Contains(content, "Microsoft.NET.Sdk.Web") {
			score += 50
			vars["isWebProject"] = true
		}

		// Parse .csproj for more details
		var csproj CsprojFile
		if err := xml.Unmarshal(data, &csproj); err == nil {
			// Get target framework
			if csproj.PropertyGroup.TargetFramework != "" {
				vars["targetFramework"] = csproj.PropertyGroup.TargetFramework
				vars["dotnetVersion"] = extractDotnetVersion(csproj.PropertyGroup.TargetFramework)
			}
		}

		// Check for ASP.NET Core packages
		if strings.Contains(content, "Microsoft.AspNetCore") {
			score += 20
		}

		// Check for Entity Framework
		if strings.Contains(content, "Microsoft.EntityFrameworkCore") {
			vars["hasEF"] = true
		}

		// Store project file name and extract project name (without .csproj extension)
		vars["projectFile"] = csprojFile
		projectName := strings.TrimSuffix(filepath.Base(csprojFile), ".csproj")
		vars["projectName"] = projectName
		break
	}

	if score == 0 {
		return 0, nil, nil
	}

	// Check for Program.cs (entry point)
	if scan.FileTree.HasFile("Program.cs") {
		score += 10
	}

	// Check for appsettings.json
	if scan.FileTree.HasFile("appsettings.json") {
		score += 10
	}

	// Check for Controllers directory (MVC pattern)
	if scan.FileTree.HasDir("Controllers") {
		score += 5
		vars["hasMVC"] = true
	}

	// Check for Pages directory (Razor Pages)
	if scan.FileTree.HasDir("Pages") {
		score += 5
		vars["hasRazorPages"] = true
	}

	// Check for solution file
	slnFiles := scan.FileTree.FilesWithExtension(".sln")
	if len(slnFiles) > 0 {
		vars["solutionFile"] = slnFiles[0]
	}

	// Store all .csproj files for multi-project restore
	allCsprojFiles := scan.FileTree.FilesWithExtension(".csproj")
	vars["allProjectFiles"] = allCsprojFiles
	vars["isMultiProject"] = len(allCsprojFiles) > 1

	// Check for Directory.Build.props (common in multi-project solutions)
	if scan.FileTree.HasFile("Directory.Build.props") {
		vars["hasDirectoryBuildProps"] = true
	}

	// Check for Directory.Packages.props (central package management)
	if scan.FileTree.HasFile("Directory.Packages.props") {
		vars["hasDirectoryPackagesProps"] = true
	}

	// Detect .NET version from global.json
	if scan.FileTree.HasFile("global.json") {
		data, err := scan.ReadFile("global.json")
		if err == nil {
			// Simple regex to extract SDK version
			re := regexp.MustCompile(`"version"\s*:\s*"(\d+\.\d+)`)
			if matches := re.FindStringSubmatch(string(data)); len(matches) > 1 {
				vars["dotnetVersion"] = matches[1]
			}
		}
	}

	// Default .NET version if not detected
	if _, ok := vars["dotnetVersion"]; !ok {
		vars["dotnetVersion"] = "8.0"
	}

	// Default port
	vars["port"] = "8080"

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// DetectVersion detects the .NET version
func (p *AspNetProvider) DetectVersion(scan *scanner.ScanResult) string {
	// Check global.json
	if scan.FileTree.HasFile("global.json") {
		data, err := scan.ReadFile("global.json")
		if err == nil {
			re := regexp.MustCompile(`"version"\s*:\s*"(\d+\.\d+)`)
			if matches := re.FindStringSubmatch(string(data)); len(matches) > 1 {
				return matches[1]
			}
		}
	}

	// Check .csproj files
	csprojFiles := scan.FileTree.FilesWithExtension(".csproj")
	for _, csprojFile := range csprojFiles {
		data, err := scan.ReadFile(csprojFile)
		if err != nil {
			continue
		}

		var csproj CsprojFile
		if err := xml.Unmarshal(data, &csproj); err == nil {
			if csproj.PropertyGroup.TargetFramework != "" {
				return extractDotnetVersion(csproj.PropertyGroup.TargetFramework)
			}
		}
	}

	return "8.0"
}

// extractDotnetVersion extracts version number from target framework
func extractDotnetVersion(targetFramework string) string {
	// Handle formats like "net8.0", "net7.0", "netcoreapp3.1"
	targetFramework = strings.ToLower(targetFramework)

	if strings.HasPrefix(targetFramework, "net") {
		version := strings.TrimPrefix(targetFramework, "net")
		version = strings.TrimPrefix(version, "coreapp")
		// Extract just major.minor
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
		if len(parts) == 1 {
			return parts[0] + ".0"
		}
	}

	return "8.0"
}
