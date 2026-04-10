package gemma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"langdag.com/langdag/types"
)

// defaultBaseURL is the Google AI Studio endpoint. Gemma models are served
// from the same generativelanguage.googleapis.com host as Gemini, so the
// base URL is shared.
const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Provider implements the provider interface for the direct Gemma API
// (Google AI Studio).
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// New creates a new Gemma provider.
func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{},
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "gemma"
}

// Models returns the available Gemma models. Includes the announced gemma-4
// family alongside the existing Gemma 3 lineup.
func (p *Provider) Models() []types.ModelInfo {
	st := []string{types.ServerToolWebSearch}
	return []types.ModelInfo{
		{ID: "gemma-4-31b-it", Name: "Gemma 4 31B", ContextWindow: 262144, MaxOutput: 8192, ServerTools: st},
		{ID: "gemma-4-26b-a4b-it", Name: "Gemma 4 26B MoE", ContextWindow: 262144, MaxOutput: 8192, ServerTools: st},
		{ID: "gemma-3-27b-it", Name: "Gemma 3 27B", ContextWindow: 131072, MaxOutput: 8192, ServerTools: st},
		{ID: "gemma-3-12b-it", Name: "Gemma 3 12B", ContextWindow: 131072, MaxOutput: 8192, ServerTools: st},
		{ID: "gemma-3-4b-it", Name: "Gemma 3 4B", ContextWindow: 131072, MaxOutput: 8192, ServerTools: st},
		{ID: "gemma-3-1b-it", Name: "Gemma 3 1B", ContextWindow: 32768, MaxOutput: 8192, ServerTools: st},
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

	var resp gemmaResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("gemma: failed to decode response: %w", err)
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
