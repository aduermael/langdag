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

	"langdag.com/langdag/types"
)

// OllamaProvider implements the provider interface for Ollama.
// Ollama is a local LLM server with an OpenAI-compatible API at /v1/chat/completions.
type OllamaProvider struct {
	baseURL            string
	apiKey             string
	client             *http.Client
	contextWindowCache sync.Map
}

func NewOllama(baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &OllamaProvider{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// NewOllamaWithAPIKey creates an Ollama provider with an optional API key.
// Useful when Ollama is deployed behind a reverse proxy that requires auth.
func NewOllamaWithAPIKey(baseURL, apiKey string) *OllamaProvider {
	p := NewOllama(baseURL)
	p.apiKey = apiKey
	return p
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type ollamaShowResponse struct {
	ModelInfo map[string]interface{} `json:"model_info"`
}


func (p *OllamaProvider) getContextWindow(modelName string) int {
	if cached, ok := p.contextWindowCache.Load(modelName); ok {
		return cached.(int)
	}

	ctx := context.Background()
	url := p.baseURL + "/api/show"

	body, _ := json.Marshal(map[string]string{"name": modelName})
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return 0
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var showResp ollamaShowResponse
	if err := json.NewDecoder(resp.Body).Decode(&showResp); err != nil {
		return 0
	}

	// model_info uses architecture-prefixed keys like "llama.context_length"
	// find any key ending in ".context_length"
	for k, v := range showResp.ModelInfo {
		if strings.HasSuffix(k, ".context_length") {
			if f, ok := v.(float64); ok {
				contextWindow := int(f)
				p.contextWindowCache.Store(modelName, contextWindow)
				return contextWindow
			}
		}
	}
	return 0
}

func (p *OllamaProvider) Models() []types.ModelInfo {
	ctx := context.Background()
	url := p.baseURL + "/api/tags"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var tagsResp ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil
	}

	models := make([]types.ModelInfo, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		models = append(models, types.ModelInfo{
			ID:            m.Name,
			Name:          m.Name,
			ContextWindow: p.getContextWindow(m.Name),
		})
	}
	return models
}

func (p *OllamaProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildRequest(req, false, nil)

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp chatCompletionResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("ollama: failed to decode response: %w", err)
	}

	return convertResponse(&resp), nil
}

func (p *OllamaProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
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

func (p *OllamaProvider) doRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama: API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp.Body, nil
}
