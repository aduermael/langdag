// Package ollama provides an Ollama provider implementation.
// Ollama uses an OpenAI-compatible API at the /v1/chat/completions endpoint.
package ollama

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

// Provider implements the provider interface for Ollama.
type Provider struct {
	baseURL string
	client  *http.Client
}

// New creates a new Ollama provider.
// The baseURL should be the Ollama server address (e.g., http://localhost:11434).
// If baseURL is empty, it defaults to http://localhost:11434.
func New(baseURL string) *Provider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Provider{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "ollama"
}

// Models returns the available models by querying the Ollama /api/tags endpoint.
func (p *Provider) Models() []types.ModelInfo {
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

	var tagsResp tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil
	}

	var models []types.ModelInfo
	for _, m := range tagsResp.Models {
		models = append(models, types.ModelInfo{
			ID:            m.Name,
			Name:          m.Name,
			ContextWindow: estimateContextWindow(m.Name),
		})
	}
	return models
}

// Complete performs a synchronous completion request.
func (p *Provider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildOllamaRequest(req, false)

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

// Stream performs a streaming completion request.
func (p *Provider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	body := buildOllamaRequest(req, true)

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

func (p *Provider) doRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	url := p.baseURL + "/v1/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer ollama")

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

// tagsResponse represents the response from /api/tags
type tagsResponse struct {
	Models []tagModel `json:"models"`
}

type tagModel struct {
	Name string `json:"name"`
}

// estimateContextWindow returns an estimated context window for Ollama models.
// Based on official Ollama library model specifications.
func estimateContextWindow(modelName string) int {
	nameLower := strings.ToLower(modelName)

	// Llama family
	switch {
	case strings.Contains(nameLower, "llama4"):
		return 1000000 // 1M context
	case strings.Contains(nameLower, "llama3.1"):
		return 128000
	case strings.Contains(nameLower, "llama3.2-vision"):
		return 128000
	case strings.Contains(nameLower, "llama3.2"):
		return 128000
	case strings.Contains(nameLower, "llama3.3"):
		return 128000
	case strings.Contains(nameLower, "llama3-gradient"):
		return 1000000 // Extended to 1M
	case strings.Contains(nameLower, "llama3"):
		return 8192
	case strings.Contains(nameLower, "llama2"):
		return 4096

	// Qwen family
	case strings.Contains(nameLower, "qwen3.5"):
		return 32768
	case strings.Contains(nameLower, "qwen3-vl"):
		return 32768
	case strings.Contains(nameLower, "qwen3-coder"):
		return 32768
	case strings.Contains(nameLower, "qwen3-embedding"):
		return 32768
	case strings.Contains(nameLower, "qwen3"):
		return 32768
	case strings.Contains(nameLower, "qwen2.5vl"):
		return 32768
	case strings.Contains(nameLower, "qwen2.5-coder"):
		return 32768
	case strings.Contains(nameLower, "qwen2.5"):
		return 32768
	case strings.Contains(nameLower, "qwen2"):
		return 32768
	case strings.Contains(nameLower, "qwen"):
		return 32768

	// Mistral family
	case strings.Contains(nameLower, "mistral-large"):
		return 128000
	case strings.Contains(nameLower, "mistral-small3"):
		return 128000
	case strings.Contains(nameLower, "mistral-nemo"):
		return 128000
	case strings.Contains(nameLower, "mistral"):
		return 32768
	case strings.Contains(nameLower, "ministral"):
		return 128000
	case strings.Contains(nameLower, "mixtral"):
		return 32768
	case strings.Contains(nameLower, "codestral"):
		return 32768
	case strings.Contains(nameLower, "mathstral"):
		return 32768
	case strings.Contains(nameLower, "magistral"):
		return 32768

	// Gemma family
	case strings.Contains(nameLower, "gemma3"):
		return 32768
	case strings.Contains(nameLower, "gemma2"):
		return 8192
	case strings.Contains(nameLower, "gemma"):
		return 8192

	// DeepSeek family
	case strings.Contains(nameLower, "deepseek-v3"):
		return 64000
	case strings.Contains(nameLower, "deepseek-r1"):
		return 64000
	case strings.Contains(nameLower, "deepseek-coder"):
		return 16384
	case strings.Contains(nameLower, "deepseek"):
		return 4096

	// Phi family
	case strings.Contains(nameLower, "phi4"):
		return 16384
	case strings.Contains(nameLower, "phi3.5"):
		return 128000
	case strings.Contains(nameLower, "phi3"):
		return 128000
	case strings.Contains(nameLower, "phi"):
		return 2048

	// LFM (Liquid) family
	case strings.Contains(nameLower, "lfm"):
		return 32768

	// GLM family
	case strings.Contains(nameLower, "glm"):
		return 32768

	// Nemotron family
	case strings.Contains(nameLower, "nemotron"):
		return 4096

	// Granite family
	case strings.Contains(nameLower, "granite"):
		return 128000

	// Command-R family
	case strings.Contains(nameLower, "command-r"):
		return 128000
	case strings.Contains(nameLower, "command-a"):
		return 128000

	// Other models with known context windows
	case strings.Contains(nameLower, "codellama"):
		return 16384
	case strings.Contains(nameLower, "starcoder"):
		return 16384
	case strings.Contains(nameLower, "yi-coder"):
		return 128000
	case strings.Contains(nameLower, "yi"):
		return 32768
	case strings.Contains(nameLower, "falcon"):
		return 2048
	case strings.Contains(nameLower, "olmo"):
		return 4096
	case strings.Contains(nameLower, "internlm"):
		return 32768
	case strings.Contains(nameLower, "dbrx"):
		return 32768
	case strings.Contains(nameLower, "solar"):
		return 4096
	case strings.Contains(nameLower, "openchat"):
		return 8192
	case strings.Contains(nameLower, "zephyr"):
		return 32768
	case strings.Contains(nameLower, "vicuna"):
		return 4096
	case strings.Contains(nameLower, "orca"):
		return 4096
	case strings.Contains(nameLower, "wizard"):
		return 4096
	case strings.Contains(nameLower, "dolphin"):
		return 8192
	case strings.Contains(nameLower, "hermes"):
		return 8192
	case strings.Contains(nameLower, "tinyllama"):
		return 2048
	case strings.Contains(nameLower, "smollm"):
		return 8192
	case strings.Contains(nameLower, "stablelm"):
		return 4096
	case strings.Contains(nameLower, "aya"):
		return 8192
	case strings.Contains(nameLower, "cogito"):
		return 32768
	case strings.Contains(nameLower, "devstral"):
		return 32768
	case strings.Contains(nameLower, "athene"):
		return 32768
	case strings.Contains(nameLower, "exaone"):
		return 32768
	case strings.Contains(nameLower, "kimi"):
		return 128000
	case strings.Contains(nameLower, "minimax"):
		return 32768
	case strings.Contains(nameLower, "tulu"):
		return 8192
	case strings.Contains(nameLower, "sailor"):
		return 32768
	case strings.Contains(nameLower, "reflection"):
		return 4096
	case strings.Contains(nameLower, "openthinker"):
		return 32768
	case strings.Contains(nameLower, "deepcoder"):
		return 32768
	case strings.Contains(nameLower, "rnj"):
		return 32768
	case strings.Contains(nameLower, "gpt-oss"):
		return 128000
	case strings.Contains(nameLower, "gemini-3"):
		return 1000000

	// Vision models
	case strings.Contains(nameLower, "llava"):
		return 4096
	case strings.Contains(nameLower, "minicpm-v"):
		return 8192
	case strings.Contains(nameLower, "bakllava"):
		return 4096
	case strings.Contains(nameLower, "moondream"):
		return 2048
	case strings.Contains(nameLower, "deepseek-ocr"):
		return 4096

	// Embedding models - typically smaller context
	case strings.Contains(nameLower, "embed") || strings.Contains(nameLower, "nomic"):
		return 8192

	default:
		return 4096 // Safe default for unknown models
	}
}
