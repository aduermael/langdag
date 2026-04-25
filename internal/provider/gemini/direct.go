package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"langdag.com/langdag/types"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Provider implements the provider interface for the direct Gemini API.
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// New creates a new Gemini provider.
func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{},
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "gemini"
}

// Models returns the available models (Gemini and Gemma families). Per-model
// capability fields (ServerTools, SupportsFunctionCalling, SupportsExplicitThinkingBudget)
// are derived from the modelCaps table so they stay consistent with the
// pre-flight validation in buildRequest.
func (p *Provider) Models() []types.ModelInfo {
	base := []types.ModelInfo{
		// Gemini models
		{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", ContextWindow: 1048576, MaxOutput: 65536},
		{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro", ContextWindow: 1048576, MaxOutput: 65536},
		{ID: "gemini-3.1-flash-lite-preview", Name: "Gemini 3.1 Flash Lite", ContextWindow: 1048576, MaxOutput: 65536},
		// Gemma models (served via the same Google AI Studio endpoint)
		{ID: "gemma-4-31b-it", Name: "Gemma 4 31B", ContextWindow: 262144, MaxOutput: 8192},
		{ID: "gemma-4-26b-a4b-it", Name: "Gemma 4 26B MoE", ContextWindow: 262144, MaxOutput: 8192},
		{ID: "gemma-3-27b-it", Name: "Gemma 3 27B", ContextWindow: 131072, MaxOutput: 8192},
		{ID: "gemma-3-12b-it", Name: "Gemma 3 12B", ContextWindow: 131072, MaxOutput: 8192},
		{ID: "gemma-3-4b-it", Name: "Gemma 3 4B", ContextWindow: 131072, MaxOutput: 8192},
		{ID: "gemma-3-1b-it", Name: "Gemma 3 1B", ContextWindow: 32768, MaxOutput: 8192},
	}
	for i := range base {
		applyCaps(&base[i])
	}
	return base
}

// applyCaps fills a ModelInfo's capability fields from modelCaps.
func applyCaps(m *types.ModelInfo) {
	c := modelCaps[m.ID]
	m.SupportsFunctionCalling = c.FunctionCalling
	m.SupportsExplicitThinkingBudget = c.ExplicitThinkingBudget
	if c.GoogleSearch {
		m.ServerTools = []string{types.ServerToolWebSearch}
	}
}

// Complete performs a synchronous completion request.
func (p *Provider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body, err := buildRequest(req)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.baseURL, req.Model, p.apiKey)

	respBody, err := doHTTPRequest(ctx, p.client, url, body, nil)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp geminiResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("gemini: failed to decode response: %w", err)
	}

	return convertResponse(&resp), nil
}

// Stream performs a streaming completion request.
func (p *Provider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	body, err := buildRequest(req)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", p.baseURL, req.Model, p.apiKey)

	respBody, err := doHTTPRequest(ctx, p.client, url, body, nil)
	if err != nil {
		return nil, err
	}

	events := make(chan types.StreamEvent, 100)
	go func() {
		defer close(events)
		defer respBody.Close()
		parseSSEStream(respBody, events)
	}()

	return events, nil
}
