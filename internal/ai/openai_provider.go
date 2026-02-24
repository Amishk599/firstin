package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// jobInsightsSchema is the JSON Schema enforced server-side via OpenAI structured outputs.
// The schema matches rawInsights exactly so the response can be parsed directly.
var jobInsightsSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"properties": map[string]any{
		"role_type": map[string]any{
			"type": "string",
			"enum": []string{
				"backend", "frontend", "fullstack", "infra",
				"SRE", "devops", "platform", "AI/ML",
				"data", "security", "mobile", "other",
			},
		},
		"years_exp": map[string]any{"type": "string"},
		"tech_stack": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"key_points": map[string]any{
			"type":     "array",
			"items":    map[string]any{"type": "string"},
			"minItems": 3,
			"maxItems": 3,
		},
	},
	"required": []string{"role_type", "years_exp", "tech_stack", "key_points"},
}

// OpenAIProvider calls the OpenAI /v1/chat/completions endpoint with structured outputs.
type OpenAIProvider struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIProvider creates a provider targeting the OpenAI API.
func NewOpenAIProvider(baseURL, apiKey, model string, httpClient *http.Client) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: httpClient,
	}
}

// chatRequest mirrors the OpenAI /v1/chat/completions request body.
type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    int            `json:"temperature"`
	MaxTokens      int            `json:"max_tokens"`
	ResponseFormat responseFormat `json:"response_format"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type       string         `json:"type"`
	JSONSchema jsonSchemaSpec `json:"json_schema"`
}

type jsonSchemaSpec struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
}

// chatResponse mirrors the relevant fields of the OpenAI response.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Complete sends prompt to OpenAI and returns a guaranteed-valid JSON string
// conforming to jobInsightsSchema. No markdown stripping required.
func (p *OpenAIProvider) Complete(ctx context.Context, prompt string) (string, error) {
	reqBody := chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: "You are a precise structured data extractor for job descriptions."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
		MaxTokens:   1024,
		ResponseFormat: responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchemaSpec{
				Name:   "job_insights",
				Schema: jobInsightsSchema,
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal llm request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read llm response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("parse llm response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("llm error (%s): %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
