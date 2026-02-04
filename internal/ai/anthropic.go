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

// AnthropicProvider implements AI generation using Anthropic Claude
type AnthropicProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &AnthropicProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.anthropic.com/v1",
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Name returns the provider name
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// IsAvailable checks if the provider is configured
func (p *AnthropicProvider) IsAvailable() bool {
	return p.apiKey != ""
}

// Generate creates Docker configuration using Anthropic Claude
func (p *AnthropicProvider) Generate(ctx context.Context, scan *scanner.ScanResult, instructions string) (*Response, error) {
	prompt := BuildPrompt(scan, instructions)

	// Build request
	reqBody := map[string]interface{}{
		"model":      p.model,
		"max_tokens": 4096,
		"system":     SystemPrompt + "\n\nIMPORTANT: Respond with valid JSON only, no markdown code blocks.",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	// Find text content
	var textContent string
	for _, c := range result.Content {
		if c.Type == "text" {
			textContent = c.Text
			break
		}
	}

	if textContent == "" {
		return nil, fmt.Errorf("no text in AI response")
	}

	// Parse the JSON response
	var response Response
	if err := json.Unmarshal([]byte(textContent), &response); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return &response, nil
}
