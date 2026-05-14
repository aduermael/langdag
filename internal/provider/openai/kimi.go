package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"langdag.com/langdag/types"
)

const kimiBaseURL = "https://api.moonshot.ai/v1"

// KimiProvider implements the provider interface for Moonshot AI's Kimi API.
// Kimi uses an OpenAI-compatible API at /v1/chat/completions with streaming
// and tool calling support.
type KimiProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client

	modelsMu      sync.Mutex
	modelCache    []types.ModelInfo
	modelsFetched bool
}

// NewKimi creates a new Kimi (Moonshot AI) provider.
// baseURL defaults to the official Moonshot API endpoint if empty.
func NewKimi(apiKey, baseURL string) *KimiProvider {
	if baseURL == "" {
		baseURL = kimiBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &KimiProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Name returns the provider name.
func (p *KimiProvider) Name() string {
	return "kimi"
}

// Models returns available models by fetching the Kimi model catalog.
// Results are cached after the first successful fetch.
func (p *KimiProvider) Models() []types.ModelInfo {
	p.modelsMu.Lock()
	defer p.modelsMu.Unlock()

	if p.modelsFetched {
		return p.modelCache
	}

	models, err := p.fetchModels()
	if err == nil {
		p.modelCache = models
		p.modelsFetched = true
	} else {
		log.Printf("kimi: failed to fetch models: %v", err)
	}
	return p.modelCache
}

// kimiModelsResponse is the shape of GET /models from the Kimi API.
type kimiModelsResponse struct {
	Data []kimiModel `json:"data"`
}

type kimiModel struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	ContextLength int    `json:"context_length"`
}

// fetchModels fetches the Kimi model catalog via GET /models.
func (p *KimiProvider) fetchModels() ([]types.ModelInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("kimi: creating models request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kimi: fetching models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return nil, fmt.Errorf("kimi: models API error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result kimiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("kimi: decoding models response: %w", err)
	}

	models := make([]types.ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, types.ModelInfo{
			ID:            m.ID,
			Name:          m.ID,
			ContextWindow: m.ContextLength,
		})
	}
	return models, nil
}

// Complete performs a synchronous completion request.
func (p *KimiProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildRequest(req, false, nil)

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp chatCompletionResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("kimi: failed to decode response: %w", err)
	}

	return convertResponse(&resp), nil
}

// Stream performs a streaming completion request.
func (p *KimiProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	body := buildRequest(req, true, nil)

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

func (p *KimiProvider) doRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("kimi: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("kimi: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return nil, fmt.Errorf("kimi: API error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	return resp.Body, nil
}
