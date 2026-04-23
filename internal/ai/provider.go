// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Provider is the interface for AI/LLM backends.
type Provider interface {
	// Complete sends a prompt and returns the model's response.
	Complete(ctx context.Context, req CompletionRequest) (string, error)
}

// CompletionRequest represents a request to the AI provider.
type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

// Config holds the configuration for the AI provider.
type Config struct {
	// Provider is the AI provider type (e.g. "openai", "ollama").
	Provider string
	// Endpoint is the API endpoint URL.
	// OpenAI: https://api.openai.com/v1
	// Ollama: http://localhost:11434/v1
	// Anthropic: https://api.anthropic.com/v1 (with compatible proxy)
	Endpoint string
	// APIKey is the API key for authenticated providers.
	APIKey string
	// Model is the model to use (e.g. "gpt-4o-mini", "llama3", "claude-sonnet-4-20250514").
	Model string
	// MaxTokens is the default max tokens per request. 0 = use provider default (1024).
	MaxTokens int
	// Temperature is the default temperature. 0 = use provider default (0.3).
	Temperature float64
}

// NewProvider creates a new AI provider from the given config.
// All providers use the OpenAI-compatible chat completions API format.
func NewProvider(cfg Config) (Provider, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("ai endpoint is required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("ai model is required")
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	temperature := cfg.Temperature
	if temperature <= 0 {
		temperature = 0.3
	}
	return &openAIProvider{
		endpoint:      cfg.Endpoint,
		apiKey:        cfg.APIKey,
		model:         cfg.Model,
		defaultMaxTok: maxTokens,
		defaultTemp:   temperature,
		client:        &http.Client{Timeout: 5 * time.Minute},
	}, nil
}

// openAIProvider implements Provider using the OpenAI-compatible chat completions API.
// This works with OpenAI, Ollama, vLLM, LiteLLM, and any OpenAI-compatible endpoint.
type openAIProvider struct {
	endpoint      string
	apiKey        string
	model         string
	defaultMaxTok int
	defaultTemp   float64
	client        *http.Client
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *openAIProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	messages := []chatMessage{}
	if req.SystemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, chatMessage{Role: "user", Content: req.UserPrompt})

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.defaultMaxTok
	}
	temperature := req.Temperature
	if temperature == 0 {
		temperature = p.defaultTemp
	}

	body := chatRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := p.endpoint + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("sending request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI provider returned %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("AI provider error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("AI provider returned no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
