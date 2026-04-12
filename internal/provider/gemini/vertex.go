package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"
	"langdag.com/langdag/types"
)

// VertexProvider implements the provider interface for Gemini via Vertex AI.
type VertexProvider struct {
	projectID string
	region    string
	baseURL   string
	client    *http.Client
}

// NewVertex creates a new Gemini Vertex AI provider.
// It uses Google Application Default Credentials for authentication.
func NewVertex(ctx context.Context, projectID, region string) (*VertexProvider, error) {
	client, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("gemini-vertex: failed to create authenticated client: %w", err)
	}

	baseURL := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s", region, projectID, region)

	return &VertexProvider{
		projectID: projectID,
		region:    region,
		baseURL:   baseURL,
		client:    client,
	}, nil
}

// Name returns the provider name.
func (p *VertexProvider) Name() string {
	return "gemini-vertex"
}

// Models returns the available models.
func (p *VertexProvider) Models() []types.ModelInfo {
	st := []string{types.ServerToolWebSearch}
	return []types.ModelInfo{
		{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash (Vertex)", ContextWindow: 1048576, MaxOutput: 65536, ServerTools: st},
		{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro (Vertex)", ContextWindow: 1048576, MaxOutput: 65536, ServerTools: st},
		{ID: "gemini-3.1-flash-lite-preview", Name: "Gemini 3.1 Flash Lite (Vertex)", ContextWindow: 1048576, MaxOutput: 65536, ServerTools: st},
	}
}

// Complete performs a synchronous completion request.
func (p *VertexProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildRequest(req)
	url := fmt.Sprintf("%s/publishers/google/models/%s:generateContent", p.baseURL, req.Model)

	respBody, err := doHTTPRequest(ctx, p.client, url, body, nil)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp geminiResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("gemini-vertex: failed to decode response: %w", err)
	}

	return convertResponse(&resp), nil
}

// Stream performs a streaming completion request.
func (p *VertexProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	body := buildRequest(req)
	url := fmt.Sprintf("%s/publishers/google/models/%s:streamGenerateContent?alt=sse", p.baseURL, req.Model)

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
