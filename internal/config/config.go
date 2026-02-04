// Package config provides configuration handling for dockerizer.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the global dockerizer configuration
type Config struct {
	// AI settings
	AI AIConfig `yaml:"ai"`

	// Default settings
	Defaults DefaultsConfig `yaml:"defaults"`

	// Provider settings
	Providers ProvidersConfig `yaml:"providers"`
}

// AIConfig contains AI provider settings
type AIConfig struct {
	Provider  string `yaml:"provider"`   // openai, anthropic, ollama
	Model     string `yaml:"model"`      // Model name
	APIKey    string `yaml:"api_key"`    // API key (can also use env var)
	BaseURL   string `yaml:"base_url"`   // Custom endpoint
	MaxTokens int    `yaml:"max_tokens"` // Max tokens for generation
	Timeout   int    `yaml:"timeout"`    // Timeout in seconds
}

// DefaultsConfig contains default generation settings
type DefaultsConfig struct {
	IncludeCompose bool   `yaml:"include_compose"`
	IncludeIgnore  bool   `yaml:"include_ignore"`
	IncludeEnv     bool   `yaml:"include_env"`
	Overwrite      bool   `yaml:"overwrite"`
	OutputDir      string `yaml:"output_dir"`
}

// ProvidersConfig contains provider-specific settings
type ProvidersConfig struct {
	MinConfidence int `yaml:"min_confidence"` // Minimum confidence threshold
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		AI: AIConfig{
			Provider:  "openai",
			Model:     "gpt-4o",
			MaxTokens: 4096,
			Timeout:   120,
		},
		Defaults: DefaultsConfig{
			IncludeCompose: true,
			IncludeIgnore:  true,
			IncludeEnv:     true,
			Overwrite:      false,
		},
		Providers: ProvidersConfig{
			MinConfidence: 80,
		},
	}
}

// Load loads configuration from the default locations
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Check for config in standard locations
	configPaths := []string{
		".dockerizer.yml",
		".dockerizer.yaml",
		filepath.Join(os.Getenv("HOME"), ".config", "dockerizer", "config.yml"),
		filepath.Join(os.Getenv("HOME"), ".dockerizer.yml"),
	}

	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			if err := cfg.loadFromFile(path); err != nil {
				return nil, err
			}
			break
		}
	}

	// Override with environment variables
	cfg.loadFromEnv()

	return cfg, nil
}

// LoadFromFile loads configuration from a specific file
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()
	if err := cfg.loadFromFile(path); err != nil {
		return nil, err
	}
	cfg.loadFromEnv()
	return cfg, nil
}

func (c *Config) loadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, c)
}

func (c *Config) loadFromEnv() {
	// AI API keys
	if key := os.Getenv("OPENAI_API_KEY"); key != "" && c.AI.Provider == "openai" {
		c.AI.APIKey = key
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" && c.AI.Provider == "anthropic" {
		c.AI.APIKey = key
	}

	// Provider override (supports both DOCKERIZER_ and legacy DOCKERIZE_ prefixes)
	if provider := os.Getenv("DOCKERIZER_AI_PROVIDER"); provider != "" {
		c.AI.Provider = provider
	} else if provider := os.Getenv("DOCKERIZE_AI_PROVIDER"); provider != "" {
		c.AI.Provider = provider
	}

	// Model override
	if model := os.Getenv("DOCKERIZER_AI_MODEL"); model != "" {
		c.AI.Model = model
	} else if model := os.Getenv("DOCKERIZE_AI_MODEL"); model != "" {
		c.AI.Model = model
	}
}

// Save writes the configuration to a file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
