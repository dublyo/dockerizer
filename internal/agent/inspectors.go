package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// SecurityInspector checks for security issues in tool calls
type SecurityInspector struct{}

func (i *SecurityInspector) Name() string { return "security" }

func (i *SecurityInspector) Inspect(ctx context.Context, tool string, args map[string]interface{}) error {
	// Check for dangerous shell commands
	if tool == "shell" {
		command, _ := args["command"].(string)

		// Block potentially dangerous commands
		dangerous := []string{
			"rm -rf /",
			"rm -rf /*",
			"dd if=",
			"mkfs",
			":(){ :|:& };:",
			"> /dev/sd",
			"chmod -R 777 /",
		}

		for _, d := range dangerous {
			if strings.Contains(command, d) {
				return fmt.Errorf("blocked dangerous command: %s", d)
			}
		}
	}

	// Check for privilege escalation in docker commands
	if tool == "docker_run" {
		if privileged, ok := args["privileged"].(bool); ok && privileged {
			return fmt.Errorf("privileged containers are not allowed")
		}
	}

	return nil
}

// SyntaxInspector validates Dockerfile syntax
type SyntaxInspector struct{}

func (i *SyntaxInspector) Name() string { return "syntax" }

func (i *SyntaxInspector) Inspect(ctx context.Context, tool string, args map[string]interface{}) error {
	if tool != "file_write" {
		return nil
	}

	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if path == "Dockerfile" || strings.HasSuffix(path, "/Dockerfile") {
		return validateDockerfileSyntax(content)
	}

	return nil
}

func validateDockerfileSyntax(content string) error {
	lines := strings.Split(content, "\n")
	hasFROM := false

	validInstructions := map[string]bool{
		"FROM": true, "RUN": true, "CMD": true, "LABEL": true,
		"EXPOSE": true, "ENV": true, "ADD": true, "COPY": true,
		"ENTRYPOINT": true, "VOLUME": true, "USER": true,
		"WORKDIR": true, "ARG": true, "ONBUILD": true,
		"STOPSIGNAL": true, "HEALTHCHECK": true, "SHELL": true,
	}

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle line continuation
		for strings.HasSuffix(line, "\\") && i+1 < len(lines) {
			i++
			line = strings.TrimSuffix(line, "\\") + " " + strings.TrimSpace(lines[i])
		}

		// Get instruction
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		instruction := strings.ToUpper(parts[0])

		// Check for FROM
		if instruction == "FROM" {
			hasFROM = true
		}

		// Validate instruction
		if !validInstructions[instruction] && !strings.HasPrefix(line, "# ") {
			// Could be a parser directive (first line only)
			if !hasFROM && strings.Contains(line, "=") {
				continue
			}
			return fmt.Errorf("invalid instruction on line %d: %s", i+1, instruction)
		}
	}

	if !hasFROM {
		return fmt.Errorf("Dockerfile must have a FROM instruction")
	}

	return nil
}

// RepetitionInspector detects repetitive patterns that might indicate a loop
type RepetitionInspector struct {
	history []string
	maxReps int
}

func NewRepetitionInspector(maxReps int) *RepetitionInspector {
	if maxReps == 0 {
		maxReps = 3
	}
	return &RepetitionInspector{
		history: make([]string, 0),
		maxReps: maxReps,
	}
}

func (i *RepetitionInspector) Name() string { return "repetition" }

func (i *RepetitionInspector) Inspect(ctx context.Context, tool string, args map[string]interface{}) error {
	// Create a signature for this call
	sig := fmt.Sprintf("%s:%v", tool, args)

	// Check for repetition
	count := 0
	for _, h := range i.history {
		if h == sig {
			count++
		}
	}

	if count >= i.maxReps {
		return fmt.Errorf("detected repetitive pattern (same call made %d times)", count)
	}

	// Add to history
	i.history = append(i.history, sig)

	// Keep history bounded
	if len(i.history) > 100 {
		i.history = i.history[50:]
	}

	return nil
}

// ContentInspector checks generated content quality
type ContentInspector struct{}

func (i *ContentInspector) Name() string { return "content" }

func (i *ContentInspector) Inspect(ctx context.Context, tool string, args map[string]interface{}) error {
	if tool != "file_write" {
		return nil
	}

	content, _ := args["content"].(string)
	path, _ := args["path"].(string)

	// Check for placeholder text that shouldn't be in final output
	placeholders := []string{
		"TODO:",
		"FIXME:",
		"YOUR_",
		"<your-",
		"{{",
		"}}",
	}

	for _, p := range placeholders {
		if strings.Contains(content, p) {
			return fmt.Errorf("content contains placeholder text: %s in %s", p, path)
		}
	}

	// Check for common typos in Dockerfiles
	if strings.HasSuffix(path, "Dockerfile") {
		typos := map[string]string{
			"FORMO":     "FROM",
			"COPPY":     "COPY",
			"EXPOES":    "EXPOSE",
			"ENTRYPOIT": "ENTRYPOINT",
			"WORKIDR":   "WORKDIR",
		}

		for typo, correct := range typos {
			re := regexp.MustCompile(`(?i)\b` + typo + `\b`)
			if re.MatchString(content) {
				return fmt.Errorf("possible typo: %s should be %s", typo, correct)
			}
		}
	}

	return nil
}
