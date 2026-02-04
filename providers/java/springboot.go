package java

import (
	"context"
	"encoding/xml"
	"strings"

	"github.com/dublyo/dockerizer/internal/scanner"
	"github.com/dublyo/dockerizer/providers"
)

// SpringBootProvider detects and generates Dockerfiles for Spring Boot projects
type SpringBootProvider struct {
	providers.BaseProvider
}

// NewSpringBootProvider creates a new Spring Boot provider
func NewSpringBootProvider() *SpringBootProvider {
	return &SpringBootProvider{
		BaseProvider: providers.BaseProvider{
			ProviderName:        "springboot",
			ProviderLanguage:    "java",
			ProviderFramework:   "springboot",
			ProviderTemplate:    "java/springboot.tmpl",
			ProviderDescription: "Spring Boot Java framework",
			ProviderURL:         "https://spring.io/projects/spring-boot",
		},
	}
}

// PomXML represents a Maven pom.xml structure
type PomXML struct {
	XMLName      xml.Name `xml:"project"`
	Parent       Parent   `xml:"parent"`
	Dependencies struct {
		Dependency []Dependency `xml:"dependency"`
	} `xml:"dependencies"`
	Properties struct {
		JavaVersion string `xml:"java.version"`
	} `xml:"properties"`
	Build struct {
		FinalName string `xml:"finalName"`
	} `xml:"build"`
}

// Parent represents the parent POM
type Parent struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

// Dependency represents a Maven dependency
type Dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
}

// Detect checks if the repository is a Spring Boot project
func (p *SpringBootProvider) Detect(ctx context.Context, scan *scanner.ScanResult) (int, map[string]interface{}, error) {
	score := 0
	vars := make(map[string]interface{})

	// Check for Maven (pom.xml) or Gradle (build.gradle)
	hasMaven := scan.FileTree.HasFile("pom.xml")
	hasGradle := scan.FileTree.HasFile("build.gradle") || scan.FileTree.HasFile("build.gradle.kts")

	if !hasMaven && !hasGradle {
		return 0, nil, nil
	}

	if hasMaven {
		vars["buildTool"] = "maven"
		score += p.detectMaven(scan, vars)
	} else if hasGradle {
		vars["buildTool"] = "gradle"
		score += p.detectGradle(scan, vars)
	}

	if score == 0 {
		return 0, nil, nil
	}

	// Check for Spring Boot application class
	if p.hasSpringBootApplication(scan) {
		score += 15
	}

	// Check for application.properties or application.yml
	if scan.FileTree.HasFile("src/main/resources/application.properties") ||
		scan.FileTree.HasFile("src/main/resources/application.yml") ||
		scan.FileTree.HasFile("src/main/resources/application.yaml") {
		score += 10
	}

	// Check for mvnw or gradlew wrapper
	if scan.FileTree.HasFile("mvnw") {
		vars["hasWrapper"] = true
	} else if scan.FileTree.HasFile("gradlew") {
		vars["hasWrapper"] = true
	}

	// Detect Java version (if not already detected from build files)
	if _, ok := vars["javaVersion"]; !ok {
		vars["javaVersion"] = detectJavaVersionFromFiles(scan)
	}

	// Default port
	vars["port"] = "8080"

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score, vars, nil
}

// detectMaven parses pom.xml for Spring Boot
func (p *SpringBootProvider) detectMaven(scan *scanner.ScanResult, vars map[string]interface{}) int {
	data, err := scan.ReadFile("pom.xml")
	if err != nil {
		return 0
	}

	var pom PomXML
	if err := xml.Unmarshal(data, &pom); err != nil {
		// Fallback to string search
		content := string(data)
		if strings.Contains(content, "spring-boot") {
			return 50
		}
		return 0
	}

	score := 0

	// Check parent for spring-boot-starter-parent
	if pom.Parent.ArtifactID == "spring-boot-starter-parent" {
		score += 50
		vars["springBootVersion"] = pom.Parent.Version
	}

	// Check dependencies for spring-boot-starter
	for _, dep := range pom.Dependencies.Dependency {
		if strings.HasPrefix(dep.ArtifactID, "spring-boot-starter") {
			score += 20
			break
		}
	}

	// Get Java version from properties
	if pom.Properties.JavaVersion != "" {
		vars["javaVersion"] = pom.Properties.JavaVersion
	}

	// Get final name
	if pom.Build.FinalName != "" {
		vars["jarName"] = pom.Build.FinalName
	}

	return score
}

// detectGradle parses build.gradle for Spring Boot
func (p *SpringBootProvider) detectGradle(scan *scanner.ScanResult, vars map[string]interface{}) int {
	var content string

	if scan.FileTree.HasFile("build.gradle.kts") {
		data, err := scan.ReadFile("build.gradle.kts")
		if err != nil {
			return 0
		}
		content = string(data)
	} else {
		data, err := scan.ReadFile("build.gradle")
		if err != nil {
			return 0
		}
		content = string(data)
	}

	score := 0

	// Check for Spring Boot plugin
	if strings.Contains(content, "org.springframework.boot") {
		score += 50
	}

	// Check for Spring Boot dependencies
	if strings.Contains(content, "spring-boot-starter") {
		score += 20
	}

	// Try to extract Java version
	if strings.Contains(content, "sourceCompatibility") || strings.Contains(content, "java.toolchain") {
		// Common patterns: sourceCompatibility = '17' or JavaVersion.VERSION_17
		if strings.Contains(content, "21") {
			vars["javaVersion"] = "21"
		} else if strings.Contains(content, "17") {
			vars["javaVersion"] = "17"
		} else if strings.Contains(content, "11") {
			vars["javaVersion"] = "11"
		}
	}

	return score
}

// hasSpringBootApplication checks for @SpringBootApplication annotation
func (p *SpringBootProvider) hasSpringBootApplication(scan *scanner.ScanResult) bool {
	// Look for Java files in src/main/java
	javaFiles := scan.FileTree.FilesWithExtension(".java")
	for _, file := range javaFiles {
		if strings.Contains(file, "Application.java") || strings.Contains(file, "App.java") {
			data, err := scan.ReadFile(file)
			if err == nil && strings.Contains(string(data), "@SpringBootApplication") {
				return true
			}
		}
	}
	return false
}

// DetectVersion detects the Java version
func (p *SpringBootProvider) DetectVersion(scan *scanner.ScanResult) string {
	return detectJavaVersionFromFiles(scan)
}

// detectJavaVersionFromFiles detects Java version from configuration files
func detectJavaVersionFromFiles(scan *scanner.ScanResult) string {
	// Check .java-version file
	if scan.FileTree.HasFile(".java-version") {
		data, err := scan.ReadFile(".java-version")
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	// Check .tool-versions (asdf)
	if scan.FileTree.HasFile(".tool-versions") {
		data, err := scan.ReadFile(".tool-versions")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "java ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						return extractJavaVersion(parts[1])
					}
				}
			}
		}
	}

	// Default to Java 21 (LTS)
	return "21"
}

// extractJavaVersion extracts version number from various formats
func extractJavaVersion(version string) string {
	// Handle formats like "temurin-21.0.2+13.0.LTS" or "21"
	version = strings.TrimPrefix(version, "temurin-")
	version = strings.TrimPrefix(version, "openjdk-")
	version = strings.TrimPrefix(version, "corretto-")
	version = strings.TrimPrefix(version, "zulu-")

	parts := strings.Split(version, ".")
	if len(parts) >= 1 {
		return parts[0]
	}
	return "21"
}
