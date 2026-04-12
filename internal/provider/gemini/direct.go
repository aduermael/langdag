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

// Models returns the available models.
func (p *Provider) Models() []types.ModelInfo {
	st := []string{types.ServerToolWebSearch}
	return []types.ModelInfo{
		{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", ContextWindow: 1048576, MaxOutput: 65536, ServerTools: st},
		{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro", ContextWindow: 1048576, MaxOutput: 65536, ServerTools: st},
		{ID: "gemini-3.1-flash-lite-preview", Name: "Gemini 3.1 Flash Lite", ContextWindow: 1048576, MaxOutput: 65536, ServerTools: st},
	}
}

// Complete performs a synchronous completion request.
func (p *Provider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildRequest(req)
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
	body := buildRequest(req)
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
