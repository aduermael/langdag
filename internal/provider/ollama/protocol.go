// Package ollama provides Ollama provider request/response handling.
// Ollama uses OpenAI-compatible API format.
package ollama

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"langdag.com/langdag/types"
)

// --- Request types (OpenAI-compatible) ---

type chatCompletionRequest struct {
	Model         string           `json:"model"`
	Messages      []requestMessage `json:"messages"`
	MaxTokens     int              `json:"max_tokens,omitempty"`
	Temperature   *float64         `json:"temperature,omitempty"`
	Stop          []string         `json:"stop,omitempty"`
	Stream        bool             `json:"stream,omitempty"`
	StreamOptions *streamOptions   `json:"stream_options,omitempty"`
	Tools         []requestTool    `json:"tools,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type requestMessage struct {
	Role       string          `json:"role"`
	Content    interface{}     `json:"content,omitempty"`
	ToolCalls  []requestToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type requestToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function requestFunction `json:"function"`
}

type requestFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type requestTool struct {
	Type     string               `json:"type"`
	Function *requestToolFunction `json:"function,omitempty"`
}

type requestToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// contentPart represents a part of multimodal content
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// --- Response types ---

type chatCompletionResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage,omitempty"`
}

type choice struct {
	Index        int             `json:"index"`
	Message      responseMessage `json:"message"`
	Delta        responseMessage `json:"delta"`
	FinishReason *string         `json:"finish_reason,omitempty"`
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
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// --- Request building ---

func buildOllamaRequest(req *types.CompletionRequest, stream bool) []byte {
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

	if system != "" {
		result = append(result, requestMessage{
			Role:    "system",
			Content: system,
		})
	}

	for _, msg := range messages {
		rm := requestMessage{Role: msg.Role}

		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			rm.Content = text
			result = append(result, rm)
			continue
		}

		var blocks []types.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			rm.Content = string(msg.Content)
			result = append(result, rm)
			continue
		}

		var toolCalls []requestToolCall
		var toolResults []requestMessage
		var contentParts []contentPart
		var hasImages bool

		for _, block := range blocks {
			switch block.Type {
			case "text":
				contentParts = append(contentParts, contentPart{Type: "text", Text: block.Text})
			case "image":
				hasImages = true
				var url string
				if block.URL != "" {
					url = block.URL
				} else if block.Data != "" {
					url = "data:" + block.MediaType + ";base64," + block.Data
				}
				if url != "" {
					contentParts = append(contentParts, contentPart{
						Type:     "image_url",
						ImageURL: &imageURL{URL: url},
					})
				}
			case "document":
				// Ollama supports text documents inline
				if block.Data != "" && block.MediaType == "text/plain" {
					contentParts = append(contentParts, contentPart{Type: "text", Text: block.Data})
				}
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
			if len(contentParts) > 0 {
				rm.Content = extractText(contentParts)
			}
			rm.ToolCalls = toolCalls
			result = append(result, rm)
			result = append(result, toolResults...)
		} else if len(toolResults) > 0 {
			result = append(result, toolResults...)
		} else if hasImages {
			rm.Content = contentParts
			result = append(result, rm)
		} else {
			rm.Content = extractText(contentParts)
			result = append(result, rm)
		}
	}

	return result
}

func extractText(parts []contentPart) string {
	var texts []string
	for _, p := range parts {
		if p.Type == "text" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func convertTools(tools []types.ToolDefinition) []requestTool {
	result := make([]requestTool, 0, len(tools))
	for _, tool := range tools {
		if tool.IsClientTool() {
			result = append(result, requestTool{
				Type: "function",
				Function: &requestToolFunction{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.InputSchema,
				},
			})
		}
	}
	return result
}

// --- Response conversion ---

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
		cr.Usage = types.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		}
	}

	return cr
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

		if chunk.Usage != nil {
			u := types.Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			finalUsage = &u
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.Content != nil && *delta.Content != "" {
			events <- types.StreamEvent{
				Type:    types.StreamEventDelta,
				Content: *delta.Content,
			}
		}

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

		if chunk.Choices[0].FinishReason != nil {
			fr := *chunk.Choices[0].FinishReason
			if fr == "tool_calls" || fr == "stop" {
				for _, tc := range currentToolCalls {
					events <- types.StreamEvent{
						Type:         types.StreamEventContentDone,
						ContentBlock: tc,
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		events <- types.StreamEvent{
			Type:  types.StreamEventError,
			Error: fmt.Errorf("ollama: stream read error: %w", err),
		}
		return
	}

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
