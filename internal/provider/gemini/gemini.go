// Package gemini provides a Google Gemini provider implementation.
package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/langdag/langdag/pkg/types"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Provider implements the provider interface for Google Gemini.
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
	return []types.ModelInfo{
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", ContextWindow: 1048576, MaxOutput: 8192},
		{ID: "gemini-2.5-pro-preview-05-06", Name: "Gemini 2.5 Pro", ContextWindow: 1048576, MaxOutput: 65536},
	}
}

// Complete performs a synchronous completion request.
func (p *Provider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildRequest(req)
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.baseURL, req.Model, p.apiKey)

	respBody, err := p.doRequest(ctx, url, body)
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

	respBody, err := p.doRequest(ctx, url, body)
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

func (p *Provider) doRequest(ctx context.Context, url string, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini: API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp.Body, nil
}

// --- Request types ---

type geminiRequest struct {
	Contents         []content         `json:"contents"`
	SystemInstruction *content         `json:"system_instruction,omitempty"`
	Tools            []tool            `json:"tools,omitempty"`
	GenerationConfig *generationConfig `json:"generation_config,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *inlineData       `json:"inlineData,omitempty"`
	FileData         *fileData         `json:"fileData,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64
}

type fileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

type functionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type functionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type tool struct {
	FunctionDeclarations []functionDeclaration `json:"function_declarations"`
}

type functionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type generationConfig struct {
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	StopSequences   []string `json:"stop_sequences,omitempty"`
}

func buildRequest(req *types.CompletionRequest) []byte {
	gr := geminiRequest{
		Contents: convertMessages(req.Messages),
	}

	if req.System != "" {
		gr.SystemInstruction = &content{
			Parts: []part{{Text: req.System}},
		}
	}

	if len(req.Tools) > 0 {
		gr.Tools = convertTools(req.Tools)
	}

	gc := &generationConfig{}
	hasConfig := false
	if req.MaxTokens > 0 {
		gc.MaxOutputTokens = req.MaxTokens
		hasConfig = true
	}
	if req.Temperature > 0 {
		gc.Temperature = &req.Temperature
		hasConfig = true
	}
	if len(req.StopSeqs) > 0 {
		gc.StopSequences = req.StopSeqs
		hasConfig = true
	}
	if hasConfig {
		gr.GenerationConfig = gc
	}

	body, _ := json.Marshal(gr)
	return body
}

func convertMessages(messages []types.Message) []content {
	var result []content

	for _, msg := range messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		} else if role != "model" {
			role = "user"
		}

		c := content{Role: role}

		// Try plain string
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			c.Parts = []part{{Text: text}}
			result = append(result, c)
			continue
		}

		// Parse content blocks
		var blocks []types.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			c.Parts = []part{{Text: string(msg.Content)}}
			result = append(result, c)
			continue
		}

		for _, block := range blocks {
			switch block.Type {
			case "text":
				c.Parts = append(c.Parts, part{Text: block.Text})
			case "image", "document":
				if block.Data != "" {
					c.Parts = append(c.Parts, part{
						InlineData: &inlineData{MimeType: block.MediaType, Data: block.Data},
					})
				} else if block.URL != "" {
					c.Parts = append(c.Parts, part{
						FileData: &fileData{MimeType: block.MediaType, FileURI: block.URL},
					})
				}
			case "tool_use":
				var args map[string]interface{}
				if block.Input != nil {
					json.Unmarshal(block.Input, &args)
				}
				c.Parts = append(c.Parts, part{
					FunctionCall: &functionCall{Name: block.Name, Args: args},
				})
			case "tool_result":
				c.Parts = append(c.Parts, part{
					FunctionResponse: &functionResponse{
						Name:     block.ToolUseID,
						Response: map[string]interface{}{"result": block.Content},
					},
				})
			}
		}

		if len(c.Parts) > 0 {
			result = append(result, c)
		}
	}

	return result
}

func convertTools(tools []types.ToolDefinition) []tool {
	var decls []functionDeclaration
	for _, t := range tools {
		decls = append(decls, functionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	return []tool{{FunctionDeclarations: decls}}
}

// --- Response types ---

type geminiResponse struct {
	Candidates    []candidate    `json:"candidates"`
	UsageMetadata *usageMetadata `json:"usageMetadata,omitempty"`
}

type candidate struct {
	Content       content `json:"content"`
	FinishReason  string  `json:"finishReason,omitempty"`
}

type usageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
}

func convertResponse(resp *geminiResponse) *types.CompletionResponse {
	cr := &types.CompletionResponse{}

	if len(resp.Candidates) > 0 {
		cand := resp.Candidates[0]
		cr.StopReason = strings.ToLower(cand.FinishReason)

		for _, p := range cand.Content.Parts {
			if p.Text != "" {
				cr.Content = append(cr.Content, types.ContentBlock{
					Type: "text",
					Text: p.Text,
				})
			}
			if p.FunctionCall != nil {
				args, _ := json.Marshal(p.FunctionCall.Args)
				cr.Content = append(cr.Content, types.ContentBlock{
					Type:  "tool_use",
					Name:  p.FunctionCall.Name,
					Input: args,
				})
			}
		}
	}

	if resp.UsageMetadata != nil {
		cr.Usage = mapUsage(resp.UsageMetadata)
	}

	return cr
}

func mapUsage(u *usageMetadata) types.Usage {
	return types.Usage{
		InputTokens:          u.PromptTokenCount,
		OutputTokens:         u.CandidatesTokenCount,
		CacheReadInputTokens: u.CachedContentTokenCount,
		ReasoningTokens:      u.ThoughtsTokenCount,
	}
}

// --- SSE streaming ---
// Gemini sends full response snapshots via SSE. We diff consecutive
// snapshots to produce text deltas.

func parseSSEStream(body io.Reader, events chan<- types.StreamEvent) {
	events <- types.StreamEvent{Type: types.StreamEventStart}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var prevText string
	var lastUsage *types.Usage

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var resp geminiResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

		if resp.UsageMetadata != nil {
			u := mapUsage(resp.UsageMetadata)
			lastUsage = &u
		}

		if len(resp.Candidates) == 0 {
			continue
		}

		cand := resp.Candidates[0]

		// Extract full text from this snapshot
		var currentText string
		for _, p := range cand.Content.Parts {
			if p.Text != "" {
				currentText += p.Text
			}
		}

		// Emit delta (diff from previous snapshot)
		if len(currentText) > len(prevText) {
			delta := currentText[len(prevText):]
			events <- types.StreamEvent{
				Type:    types.StreamEventDelta,
				Content: delta,
			}
			prevText = currentText
		}

		// Emit function calls when they appear
		for _, p := range cand.Content.Parts {
			if p.FunctionCall != nil {
				args, _ := json.Marshal(p.FunctionCall.Args)
				events <- types.StreamEvent{
					Type: types.StreamEventContentDone,
					ContentBlock: &types.ContentBlock{
						Type:  "tool_use",
						Name:  p.FunctionCall.Name,
						Input: args,
					},
				}
			}
		}
	}

	resp := &types.CompletionResponse{}
	if lastUsage != nil {
		resp.Usage = *lastUsage
	}

	events <- types.StreamEvent{
		Type:     types.StreamEventDone,
		Response: resp,
	}
}
