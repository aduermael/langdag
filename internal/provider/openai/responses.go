// Package openai — Responses API support for OpenAI-compatible providers.
//
// This file contains request building, response parsing, and SSE streaming for
// the Responses API (/v1/responses). The Responses API is the modern endpoint
// used by xAI/Grok (and increasingly by OpenAI) that supports server-side tools
// such as web_search, x_search, and code_interpreter alongside function calling.
package openai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"langdag.com/langdag/types"
)

// --- Responses API request types ---

type responsesRequest struct {
	Model           string        `json:"model"`
	Input           []interface{} `json:"input"`
	Instructions    string        `json:"instructions,omitempty"`
	Tools           []interface{} `json:"tools,omitempty"`
	MaxOutputTokens int           `json:"max_output_tokens,omitempty"`
	Temperature     *float64      `json:"temperature,omitempty"`
	Stream          bool          `json:"stream,omitempty"`
	Store           bool          `json:"store"`
}

// Input item types for the Responses API.

type responsesInputMessage struct {
	Type    string      `json:"type"`
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []responsesContentPart
}

type responsesInputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesInputImage struct {
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
}

type responsesOutputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesFunctionCallInput struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Status    string `json:"status,omitempty"`
}

type responsesFunctionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// Tool types for the Responses API.

type responsesFunctionTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type responsesServerTool struct {
	Type string `json:"type"`
}

// --- Responses API response types ---

type responsesResponse struct {
	ID     string            `json:"id"`
	Model  string            `json:"model"`
	Output []responsesOutput `json:"output"`
	Usage  *responsesUsage   `json:"usage,omitempty"`
	Status string            `json:"status"`
}

type responsesOutput struct {
	Type string `json:"type"`

	// For "message" type
	ID      string                  `json:"id,omitempty"`
	Role    string                  `json:"role,omitempty"`
	Content []responsesContentBlock `json:"content,omitempty"`
	Status  string                  `json:"status,omitempty"`

	// For "function_call" type
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type responsesContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type responsesUsage struct {
	InputTokens         int                     `json:"input_tokens"`
	OutputTokens        int                     `json:"output_tokens"`
	OutputTokensDetails *responsesTokensDetails `json:"output_tokens_details,omitempty"`
}

type responsesTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// --- Responses API streaming event ---

type responsesStreamEvent struct {
	Type string `json:"type"`

	// For response.output_text.delta
	Delta string `json:"delta,omitempty"`

	// For response.output_item.added
	Item *responsesOutput `json:"item,omitempty"`

	// For response.completed
	Response *responsesResponse `json:"response,omitempty"`

	// Common fields
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index,omitempty"`
	ItemID       string `json:"item_id,omitempty"`
}

// --- Request building ---

func buildResponsesRequest(req *types.CompletionRequest, stream bool) []byte {
	instructions, input := convertResponsesMessages(req.Messages, req.System)

	rr := responsesRequest{
		Model:        req.Model,
		Input:        input,
		Instructions: instructions,
		Stream:       stream,
		Store:        false,
	}

	if req.MaxTokens > 0 {
		rr.MaxOutputTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		rr.Temperature = &req.Temperature
	}
	if len(req.Tools) > 0 {
		rr.Tools = convertResponsesTools(req.Tools)
	}

	body, _ := json.Marshal(rr)
	return body
}

// convertResponsesMessages converts langdag messages to Responses API input items.
// The system prompt is returned separately as the "instructions" field.
func convertResponsesMessages(messages []types.Message, system string) (string, []interface{}) {
	var input []interface{}

	for _, msg := range messages {
		// Try plain text first
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			input = append(input, responsesInputMessage{
				Type:    "message",
				Role:    msg.Role,
				Content: text,
			})
			continue
		}

		// Parse as content blocks
		var blocks []types.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			// Fallback: treat raw content as text
			input = append(input, responsesInputMessage{
				Type:    "message",
				Role:    msg.Role,
				Content: string(msg.Content),
			})
			continue
		}

		// Separate blocks by type
		var textParts []interface{}
		var toolCalls []responsesFunctionCallInput
		var toolResults []responsesFunctionCallOutput
		var hasImages bool

		for _, block := range blocks {
			switch block.Type {
			case "text":
				textParts = append(textParts, responsesInputText{
					Type: "input_text",
					Text: block.Text,
				})
			case "image":
				hasImages = true
				var url string
				if block.URL != "" {
					url = block.URL
				} else if block.Data != "" {
					url = "data:" + block.MediaType + ";base64," + block.Data
				}
				if url != "" {
					textParts = append(textParts, responsesInputImage{
						Type:     "input_image",
						ImageURL: url,
					})
				}
			case "tool_use":
				toolCalls = append(toolCalls, responsesFunctionCallInput{
					Type:      "function_call",
					CallID:    block.ID,
					Name:      block.Name,
					Arguments: string(block.Input),
					Status:    "completed",
				})
			case "tool_result":
				content := block.Content
				if len(block.ContentJSON) > 0 {
					content = string(block.ContentJSON)
				}
				toolResults = append(toolResults, responsesFunctionCallOutput{
					Type:   "function_call_output",
					CallID: block.ToolUseID,
					Output: content,
				})
			}
		}

		// Emit items in the order expected by the Responses API:
		// 1. Assistant message text (if any)
		// 2. Function calls
		// 3. Function call outputs

		if len(toolCalls) > 0 {
			// Assistant message with tool calls
			if len(textParts) > 0 {
				input = append(input, responsesInputMessage{
					Type:    "message",
					Role:    "assistant",
					Content: textParts,
				})
			}
			for _, tc := range toolCalls {
				input = append(input, tc)
			}
			for _, tr := range toolResults {
				input = append(input, tr)
			}
		} else if len(toolResults) > 0 {
			// Tool results without associated tool calls in this message
			for _, tr := range toolResults {
				input = append(input, tr)
			}
		} else if hasImages {
			input = append(input, responsesInputMessage{
				Type:    "message",
				Role:    msg.Role,
				Content: textParts,
			})
		} else {
			// Plain text from content blocks
			var texts []string
			for _, block := range blocks {
				if block.Type == "text" {
					texts = append(texts, block.Text)
				}
			}
			input = append(input, responsesInputMessage{
				Type:    "message",
				Role:    msg.Role,
				Content: strings.Join(texts, "\n"),
			})
		}
	}

	return system, input
}

// convertResponsesTools converts langdag tool definitions to Responses API tool params.
func convertResponsesTools(tools []types.ToolDefinition) []interface{} {
	result := make([]interface{}, 0, len(tools))
	for _, tool := range tools {
		if tool.IsClientTool() {
			result = append(result, responsesFunctionTool{
				Type:        "function",
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			})
			continue
		}

		// Server-side tool: the standardized name maps directly to the
		// Responses API type (e.g. "web_search" → {"type": "web_search"}).
		result = append(result, responsesServerTool{Type: tool.Name})
	}
	return result
}

// --- Response conversion ---

func convertResponsesResult(resp *responsesResponse) *types.CompletionResponse {
	cr := &types.CompletionResponse{
		ID:    resp.ID,
		Model: resp.Model,
	}

	hasFunctionCalls := false
	for _, out := range resp.Output {
		switch out.Type {
		case "message":
			for _, part := range out.Content {
				if part.Type == "output_text" && part.Text != "" {
					cr.Content = append(cr.Content, types.ContentBlock{
						Type: "text",
						Text: part.Text,
					})
				}
			}
		case "function_call":
			hasFunctionCalls = true
			cr.Content = append(cr.Content, types.ContentBlock{
				Type:  "tool_use",
				ID:    out.CallID,
				Name:  out.Name,
				Input: json.RawMessage(out.Arguments),
			})
		}
	}

	if hasFunctionCalls {
		cr.StopReason = "tool_calls"
	} else {
		cr.StopReason = "stop"
	}

	if resp.Usage != nil {
		cr.Usage = types.Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		}
		if resp.Usage.OutputTokensDetails != nil {
			cr.Usage.ReasoningTokens = resp.Usage.OutputTokensDetails.ReasoningTokens
		}
	}

	return cr
}

// --- SSE streaming ---

func parseResponsesSSEStream(body io.Reader, events chan<- types.StreamEvent) {
	events <- types.StreamEvent{Type: types.StreamEventStart}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	currentFunctionCalls := map[int]*types.ContentBlock{}

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "response.output_text.delta":
			events <- types.StreamEvent{
				Type:    types.StreamEventDelta,
				Content: event.Delta,
			}

		case "response.output_item.added":
			// Track new function call items
			if event.Item != nil && event.Item.Type == "function_call" {
				currentFunctionCalls[event.OutputIndex] = &types.ContentBlock{
					Type: "tool_use",
					ID:   event.Item.CallID,
					Name: event.Item.Name,
				}
			}

		case "response.function_call_arguments.delta":
			existing, ok := currentFunctionCalls[event.OutputIndex]
			if !ok {
				existing = &types.ContentBlock{Type: "tool_use"}
				currentFunctionCalls[event.OutputIndex] = existing
			}
			if event.Delta != "" {
				var prev string
				if existing.Input != nil {
					prev = string(existing.Input)
				}
				existing.Input = json.RawMessage(prev + event.Delta)
			}

		case "response.function_call_arguments.done":
			if cb, ok := currentFunctionCalls[event.OutputIndex]; ok {
				events <- types.StreamEvent{
					Type:         types.StreamEventContentDone,
					ContentBlock: cb,
				}
			}

		case "response.completed":
			if event.Response != nil {
				resp := convertResponsesResult(event.Response)
				events <- types.StreamEvent{
					Type:     types.StreamEventDone,
					Response: resp,
				}
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		events <- types.StreamEvent{
			Type:  types.StreamEventError,
			Error: fmt.Errorf("responses: stream read error: %w", err),
		}
		return
	}

	// Fallback: build a minimal response from accumulated function calls.
	var content []types.ContentBlock
	for _, cb := range currentFunctionCalls {
		content = append(content, *cb)
	}
	stopReason := "stop"
	if len(content) > 0 {
		stopReason = "tool_calls"
	}

	events <- types.StreamEvent{
		Type: types.StreamEventDone,
		Response: &types.CompletionResponse{
			Content:    content,
			StopReason: stopReason,
		},
	}
}
