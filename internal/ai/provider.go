// Package ai provides AI provider integration for dockerizer.
package ai

import (
	"context"
	"fmt"

	"github.com/dublyo/dockerizer/internal/scanner"
)

// Provider generates Dockerfiles using AI
type Provider interface {
	Name() string
	Generate(ctx context.Context, scan *scanner.ScanResult, instructions string) (*Response, error)
	IsAvailable() bool
}

// Response from AI provider
type Response struct {
	Dockerfile    string   `json:"dockerfile"`
	DockerCompose string   `json:"docker_compose"`
	Dockerignore  string   `json:"dockerignore"`
	EnvExample    string   `json:"env_example"`
	Explanation   string   `json:"explanation"`
	Warnings      []string `json:"warnings"`
}

// Config for AI providers
type Config struct {
	Provider  string `json:"provider"` // "openai" or "anthropic" or "ollama"
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	BaseURL   string `json:"base_url"` // For custom endpoints
}

// NewProvider creates a new AI provider based on config
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIProvider(cfg.APIKey, cfg.Model), nil
	case "anthropic":
		return NewAnthropicProvider(cfg.APIKey, cfg.Model), nil
	case "ollama":
		return NewOllamaProvider(cfg.BaseURL, cfg.Model), nil
	default:
		return nil, fmt.Errorf("unknown AI provider: %s", cfg.Provider)
	}
}

// SystemPrompt is the system prompt for AI generation
const SystemPrompt = `You are an expert DevOps engineer specializing in Docker containerization.
Your task is to generate production-ready Docker configurations for any project.

Guidelines:
1. Generate multi-stage Dockerfiles when beneficial
2. Use specific version tags, never :latest
3. Create non-root users for security
4. Include proper health checks
5. Optimize layer caching (copy dependencies before source)
6. Minimize final image size
7. Use .dockerignore to exclude unnecessary files
8. Include resource limits in docker-compose
9. Configure proper logging
10. Add Traefik labels for reverse proxy integration

Output format: Respond with a JSON object containing:
- dockerfile: The Dockerfile content (string)
- docker_compose: The docker-compose.yml content (string)
- dockerignore: The .dockerignore content (string)
- env_example: The .env.example content (string)
- explanation: Brief explanation of choices made (string)
- warnings: Array of strings with any issues (e.g., ["warning1", "warning2"] or [] if none)

IMPORTANT: Always respond with valid JSON only. No markdown. The warnings field MUST be an array.`

// BuildPrompt constructs the prompt for AI generation
func BuildPrompt(scan *scanner.ScanResult, instructions string) string {
	prompt := "Generate Docker configuration for this project:\n\n"

	// Add file tree
	prompt += "## Project Structure\n```\n"
	for _, f := range scan.FileTree.Files {
		prompt += f + "\n"
	}
	prompt += "```\n\n"

	// Add key files content
	prompt += "## Key Files\n"
	for _, kf := range scan.KeyFiles {
		prompt += fmt.Sprintf("### %s\n```\n%s\n```\n\n", kf.Path, kf.Content)
	}

	// Add user instructions if provided
	if instructions != "" {
		prompt += fmt.Sprintf("## Additional Instructions\n%s\n", instructions)
	}

	return prompt
}
