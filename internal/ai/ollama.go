package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dublyo/dockerizer/internal/scanner"
)

// OllamaProvider implements AI generation using Ollama (local LLM)
type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3.1"
	}
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 300 * time.Second, // Longer timeout for local models
		},
	}
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// IsAvailable checks if Ollama is running
func (p *OllamaProvider) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Generate creates Docker configuration using Ollama
func (p *OllamaProvider) Generate(ctx context.Context, scan *scanner.ScanResult, instructions string) (*Response, error) {
	prompt := BuildPrompt(scan, instructions)

	// Build request
	reqBody := map[string]interface{}{
		"model":  p.model,
		"prompt": SystemPrompt + "\n\n" + prompt + "\n\nRespond with valid JSON only.",
		"stream": false,
		"format": "json",
		"options": map[string]interface{}{
			"temperature": 0.2,
			"num_predict": 4096,
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Response string `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Response == "" {
		return nil, fmt.Errorf("empty response from Ollama")
	}

	// Parse the JSON response
	var response Response
	if err := json.Unmarshal([]byte(result.Response), &response); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return &response, nil
}
