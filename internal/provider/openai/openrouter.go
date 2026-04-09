package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"langdag.com/langdag/types"
)

const openRouterBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterProvider implements the provider interface for OpenRouter.
// OpenRouter is an OpenAI-compatible API that routes to many underlying models.
// It requires HTTP-Referer and X-Title headers on every request.
type OpenRouterProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client

	modelsMu    sync.Mutex
	modelCache  []types.ModelInfo
	modelsFetched bool
}

// NewOpenRouter creates a new OpenRouter provider.
// baseURL defaults to the official OpenRouter endpoint if empty.
func NewOpenRouter(apiKey, baseURL string) *OpenRouterProvider {
	if baseURL == "" {
		baseURL = openRouterBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &OpenRouterProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Name returns the provider name.
func (p *OpenRouterProvider) Name() string {
	return "openrouter"
}

// Models returns available models by fetching the OpenRouter model catalog.
// Results are cached after the first successful fetch.
func (p *OpenRouterProvider) Models() []types.ModelInfo {
	p.modelsMu.Lock()
	defer p.modelsMu.Unlock()

	if p.modelsFetched {
		return p.modelCache
	}

	models, err := p.fetchModels()
	if err == nil {
		p.modelCache = models
	}
	p.modelsFetched = true
	return p.modelCache
}

// openRouterModelsResponse is the shape of GET /models from OpenRouter.
type openRouterModelsResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	TopProvider   struct {
		MaxCompletionTokens int `json:"max_completion_tokens"`
	} `json:"top_provider"`
}

// fetchModels fetches the OpenRouter model catalog via GET /models.
// OpenRouter requires HTTP-Referer and X-Title headers on every request,
// including this one.
func (p *OpenRouterProvider) fetchModels() ([]types.ModelInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("openrouter: creating models request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/aduermael/langdag")
	req.Header.Set("X-Title", "langdag")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: fetching models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter: models API error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result openRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openrouter: decoding models response: %w", err)
	}

	models := make([]types.ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, types.ModelInfo{
			ID:            m.ID,
			Name:          m.Name,
			ContextWindow: m.ContextLength,
			MaxOutput:     m.TopProvider.MaxCompletionTokens,
		})
	}
	return models, nil
}

// Complete performs a synchronous completion request.
func (p *OpenRouterProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildRequest(req, false, openAIServerTools)

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp chatCompletionResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("openrouter: failed to decode response: %w", err)
	}

	return convertResponse(&resp), nil
}

// Stream performs a streaming completion request.
func (p *OpenRouterProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	body := buildRequest(req, true, openAIServerTools)

	respBody, err := p.doRequest(ctx, body)
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

func (p *OpenRouterProvider) doRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openrouter: creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/aduermael/langdag")
	httpReq.Header.Set("X-Title", "langdag")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter: API error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	return resp.Body, nil
}
