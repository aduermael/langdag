package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"langdag.com/langdag/types"
)

// GrokProvider implements the provider interface for xAI's Grok API.
// Grok uses the Responses API (/v1/responses) which supports server-side tools
// like web_search and x_search alongside function calling.
type GrokProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewGrok creates a new Grok (xAI) provider.
func NewGrok(apiKey, baseURL string) *GrokProvider {
	if baseURL == "" {
		baseURL = "https://api.x.ai/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &GrokProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Name returns the provider name.
func (p *GrokProvider) Name() string {
	return "grok"
}

// Models returns the available Grok models.
func (p *GrokProvider) Models() []types.ModelInfo {
	return []types.ModelInfo{
		{ID: "grok-3", Name: "Grok 3", ContextWindow: 131072, MaxOutput: 16384},
		{ID: "grok-3-mini", Name: "Grok 3 Mini", ContextWindow: 131072, MaxOutput: 16384},
	}
}

// Complete performs a synchronous completion request using the Responses API.
func (p *GrokProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildResponsesRequest(req, false)

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp responsesResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("grok: failed to decode response: %w", err)
	}

	return convertResponsesResult(&resp), nil
}

// Stream performs a streaming completion request using the Responses API.
func (p *GrokProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	body := buildResponsesRequest(req, true)

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	events := make(chan types.StreamEvent, 100)
	go func() {
		defer close(events)
		defer respBody.Close()
		parseResponsesSSEStream(respBody, events)
	}()

	return events, nil
}

func (p *GrokProvider) doRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("grok: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("grok: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("grok: API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp.Body, nil
}
