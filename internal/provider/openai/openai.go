// Package openai provides an OpenAI-compatible provider implementation.
// It works with OpenAI, xAI/Grok, Mistral, and any API that follows
// the OpenAI chat completions format.
package openai

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

// Provider implements the provider interface for OpenAI-compatible APIs.
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// New creates a new OpenAI-compatible provider.
func New(apiKey, baseURL string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Provider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "openai"
}

// Models returns the available models.
func (p *Provider) Models() []types.ModelInfo {
	return []types.ModelInfo{
		{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000, MaxOutput: 16384},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ContextWindow: 128000, MaxOutput: 16384},
		{ID: "o3-mini", Name: "o3-mini", ContextWindow: 200000, MaxOutput: 100000},
	}
}

// Complete performs a synchronous completion request.
func (p *Provider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	body := buildRequest(req, false)

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	var resp chatCompletionResponse
	if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
		return nil, fmt.Errorf("openai: failed to decode response: %w", err)
	}

	return convertResponse(&resp), nil
}

// Stream performs a streaming completion request.
func (p *Provider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	body := buildRequest(req, true)

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
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp.Body, nil
}

// --- Request building ---

type chatCompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []requestMessage `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	Stop        []string         `json:"stop,omitempty"`
	Tools       []requestTool    `json:"tools,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type requestMessage struct {
	Role       string             `json:"role"`
	Content    interface{}        `json:"content,omitempty"` // string or []contentPart
	ToolCalls  []requestToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type requestToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function requestFunction  `json:"function"`
}

type requestFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type requestTool struct {
	Type     string              `json:"type"`
	Function requestToolFunction `json:"function"`
}

type requestToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func buildRequest(req *types.CompletionRequest, stream bool) []byte {
	messages := convertMessages(req.Messages, req.System)

	cr := chatCompletionRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   stream,
	}

	if req.MaxTokens > 0 {
		cr.MaxTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		cr.Temperature = &req.Temperature
	}
	if len(req.StopSeqs) > 0 {
		cr.Stop = req.StopSeqs
	}
	if len(req.Tools) > 0 {
		cr.Tools = convertTools(req.Tools)
	}
	if stream {
		cr.StreamOptions = &streamOptions{IncludeUsage: true}
	}

	body, _ := json.Marshal(cr)
	return body
}

func convertMessages(messages []types.Message, system string) []requestMessage {
	var result []requestMessage

	// System prompt as a system role message
	if system != "" {
		result = append(result, requestMessage{
			Role:    "system",
			Content: system,
		})
	}

	for _, msg := range messages {
		rm := requestMessage{Role: msg.Role}

		// Try to parse as plain string
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			rm.Content = text
			result = append(result, rm)
			continue
		}

		// Parse as content blocks
		var blocks []types.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			rm.Content = string(msg.Content)
			result = append(result, rm)
			continue
		}

		// Check if blocks contain tool_use (assistant with tool calls)
		var toolCalls []requestToolCall
		var textParts []string
		var toolResults []requestMessage

		for _, block := range blocks {
			switch block.Type {
			case "text":
				textParts = append(textParts, block.Text)
			case "tool_use":
				toolCalls = append(toolCalls, requestToolCall{
					ID:   block.ID,
					Type: "function",
					Function: requestFunction{
						Name:      block.Name,
						Arguments: string(block.Input),
					},
				})
			case "tool_result":
				toolResults = append(toolResults, requestMessage{
					Role:       "tool",
					Content:    block.Content,
					ToolCallID: block.ToolUseID,
				})
			}
		}

		if len(toolCalls) > 0 {
			rm.Role = "assistant"
			if len(textParts) > 0 {
				rm.Content = strings.Join(textParts, "\n")
			}
			rm.ToolCalls = toolCalls
			result = append(result, rm)
			result = append(result, toolResults...)
		} else if len(toolResults) > 0 {
			result = append(result, toolResults...)
		} else {
			rm.Content = strings.Join(textParts, "\n")
			result = append(result, rm)
		}
	}

	return result
}

func convertTools(tools []types.ToolDefinition) []requestTool {
	result := make([]requestTool, len(tools))
	for i, tool := range tools {
		result[i] = requestTool{
			Type: "function",
			Function: requestToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		}
	}
	return result
}

// --- Response parsing ---

type chatCompletionResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage,omitempty"`
}

type choice struct {
	Index        int              `json:"index"`
	Message      responseMessage  `json:"message"`
	Delta        responseMessage  `json:"delta"`
	FinishReason *string          `json:"finish_reason,omitempty"`
}

type responseMessage struct {
	Role      string             `json:"role,omitempty"`
	Content   *string            `json:"content,omitempty"`
	ToolCalls []responseToolCall `json:"tool_calls,omitempty"`
}

type responseToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Index    int              `json:"index"`
	Function responseFunction `json:"function"`
}

type responseFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type usage struct {
	PromptTokens            int           `json:"prompt_tokens"`
	CompletionTokens        int           `json:"completion_tokens"`
	PromptTokensDetails     *tokenDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *tokenDetails `json:"completion_tokens_details,omitempty"`
}

type tokenDetails struct {
	CachedTokens    int `json:"cached_tokens,omitempty"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

func convertResponse(resp *chatCompletionResponse) *types.CompletionResponse {
	cr := &types.CompletionResponse{
		ID:    resp.ID,
		Model: resp.Model,
	}

	if len(resp.Choices) > 0 {
		c := resp.Choices[0]
		if c.FinishReason != nil {
			cr.StopReason = *c.FinishReason
		}

		if c.Message.Content != nil {
			cr.Content = append(cr.Content, types.ContentBlock{
				Type: "text",
				Text: *c.Message.Content,
			})
		}

		for _, tc := range c.Message.ToolCalls {
			cr.Content = append(cr.Content, types.ContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(tc.Function.Arguments),
			})
		}
	}

	if resp.Usage != nil {
		cr.Usage = mapUsage(resp.Usage)
	}

	return cr
}

func mapUsage(u *usage) types.Usage {
	result := types.Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
	if u.PromptTokensDetails != nil {
		result.CacheReadInputTokens = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		result.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return result
}

// --- SSE streaming ---

func parseSSEStream(body io.Reader, events chan<- types.StreamEvent) {
	events <- types.StreamEvent{Type: types.StreamEventStart}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentToolCalls = map[int]*types.ContentBlock{}
	var finalUsage *types.Usage
	var responseID, responseModel string

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var chunk chatCompletionResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.ID != "" {
			responseID = chunk.ID
		}
		if chunk.Model != "" {
			responseModel = chunk.Model
		}

		// Usage chunk (sent with stream_options.include_usage)
		if chunk.Usage != nil {
			u := mapUsage(chunk.Usage)
			finalUsage = &u
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// Text delta
		if delta.Content != nil && *delta.Content != "" {
			events <- types.StreamEvent{
				Type:    types.StreamEventDelta,
				Content: *delta.Content,
			}
		}

		// Tool call deltas
		for _, tc := range delta.ToolCalls {
			existing, ok := currentToolCalls[tc.Index]
			if !ok {
				existing = &types.ContentBlock{
					Type: "tool_use",
					ID:   tc.ID,
					Name: tc.Function.Name,
				}
				currentToolCalls[tc.Index] = existing
			} else {
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Name += tc.Function.Name
				}
			}
			if tc.Function.Arguments != "" {
				var prev string
				if existing.Input != nil {
					prev = string(existing.Input)
				}
				existing.Input = json.RawMessage(prev + tc.Function.Arguments)
			}
		}

		// Check for finish reason
		if chunk.Choices[0].FinishReason != nil {
			fr := *chunk.Choices[0].FinishReason
			if fr == "tool_calls" || fr == "stop" {
				// Emit completed tool calls
				for _, tc := range currentToolCalls {
					events <- types.StreamEvent{
						Type:         types.StreamEventContentDone,
						ContentBlock: tc,
					}
				}
			}
		}
	}

	// Build final response
	var content []types.ContentBlock
	for _, tc := range currentToolCalls {
		content = append(content, *tc)
	}

	resp := &types.CompletionResponse{
		ID:      responseID,
		Model:   responseModel,
		Content: content,
	}
	if finalUsage != nil {
		resp.Usage = *finalUsage
	}

	events <- types.StreamEvent{
		Type:     types.StreamEventDone,
		Response: resp,
	}
}
