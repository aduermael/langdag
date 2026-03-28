// Package anthropic provides Anthropic-protocol implementations of the provider interface.
// This file contains shared message/tool conversion, response parsing, and stream
// event handling used by all Anthropic-protocol variants (direct, Vertex, Bedrock).
package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"langdag.com/langdag/types"
)

// buildParams constructs the common MessageNewParams from a CompletionRequest.
func buildParams(req *types.CompletionRequest) (anthropic.MessageNewParams, error) {
	messages, err := convertMessages(req.Messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  messages,
		MaxTokens: int64(req.MaxTokens),
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{
				Text:         req.System,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		}
	}

	// Extended thinking: when explicitly enabled, set the thinking config
	// and adjust max_tokens so the budget fits within the limit.
	if req.Think != nil && *req.Think {
		const thinkingBudget int64 = 10240
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(thinkingBudget)
		// Anthropic requires max_tokens > budget_tokens; ensure enough room
		// for actual output on top of the thinking budget.
		if minTokens := thinkingBudget + 4096; params.MaxTokens < minTokens {
			params.MaxTokens = minTokens
		}
		// Temperature must NOT be set when thinking is enabled.
		// Skip the temperature block below.
	} else if req.Temperature > 0 {
		params.Temperature = param.NewOpt(req.Temperature)
	}

	if len(req.StopSeqs) > 0 {
		params.StopSequences = req.StopSeqs
	}

	if len(req.Tools) > 0 {
		tools, err := convertTools(req.Tools)
		if err != nil {
			return anthropic.MessageNewParams{}, err
		}
		// Set cache control breakpoint on the last tool for prompt caching.
		// Anthropic caches everything up to and including the marked block,
		// so subsequent requests with identical tools pay only 10% of input cost.
		if n := len(tools); n > 0 {
			if cc := tools[n-1].GetCacheControl(); cc != nil {
				*cc = anthropic.NewCacheControlEphemeralParam()
			}
		}
		params.Tools = tools
	}

	return params, nil
}

// convertMessages converts types.Message to anthropic message params.
func convertMessages(messages []types.Message) ([]anthropic.MessageParam, error) {
	result := make([]anthropic.MessageParam, 0, len(messages))

	for _, msg := range messages {
		var content string
		if err := json.Unmarshal(msg.Content, &content); err != nil {
			// Try to parse as content blocks
			var blocks []types.ContentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err != nil {
				return nil, fmt.Errorf("failed to parse message content: %w", err)
			}

			// Convert content blocks
			anthropicBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(blocks))
			for _, block := range blocks {
				switch block.Type {
				case "text":
					if block.Text == "" {
						continue
					}
					anthropicBlocks = append(anthropicBlocks, anthropic.NewTextBlock(block.Text))
				case "image":
					if block.Data != "" {
						anthropicBlocks = append(anthropicBlocks, anthropic.NewImageBlockBase64(block.MediaType, block.Data))
					} else if block.URL != "" {
						anthropicBlocks = append(anthropicBlocks, anthropic.NewImageBlock(anthropic.URLImageSourceParam{URL: block.URL}))
					}
				case "document":
					if block.Data != "" {
						var source anthropic.DocumentBlockParamSourceUnion
						switch block.MediaType {
						case "text/plain":
							source = anthropic.DocumentBlockParamSourceUnion{
								OfText: &anthropic.PlainTextSourceParam{
									Data: block.Data,
								},
							}
						default: // "application/pdf" or unspecified
							source = anthropic.DocumentBlockParamSourceUnion{
								OfBase64: &anthropic.Base64PDFSourceParam{
									Data: block.Data,
								},
							}
						}
						anthropicBlocks = append(anthropicBlocks, anthropic.ContentBlockParamUnion{
							OfDocument: &anthropic.DocumentBlockParam{
								Source: source,
							},
						})
					} else if block.URL != "" {
						anthropicBlocks = append(anthropicBlocks, anthropic.ContentBlockParamUnion{
							OfDocument: &anthropic.DocumentBlockParam{
								Source: anthropic.DocumentBlockParamSourceUnion{
									OfURL: &anthropic.URLPDFSourceParam{
										URL: block.URL,
									},
								},
							},
						})
					}
				case "tool_use":
					var input interface{}
					if block.Input != nil {
						_ = json.Unmarshal(block.Input, &input)
					}
					anthropicBlocks = append(anthropicBlocks, anthropic.NewToolUseBlock(block.ID, input, block.Name))
				case "tool_result":
					anthropicBlocks = append(anthropicBlocks, anthropic.NewToolResultBlock(block.ToolUseID, block.Content, block.IsError))
				}
			}

			// Skip messages with no usable content blocks (e.g. all empty text blocks
			// filtered out). Sending an empty content array causes a 400 API error.
			if len(anthropicBlocks) > 0 {
				result = append(result, anthropic.MessageParam{
					Role:    anthropic.MessageParamRole(msg.Role),
					Content: anthropicBlocks,
				})
			}
		} else {
			// Skip empty-text assistant messages — these arise from max_tokens
			// truncation saving an empty node. Sending {"type":"text","text":""}
			// causes a 400 API error.
			if content == "" && msg.Role == "assistant" {
				continue
			}
			if msg.Role == "assistant" {
				result = append(result, anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
			} else {
				result = append(result, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
			}
		}
	}

	return result, nil
}

// anthropicServerTools maps standardized tool names to Anthropic-specific
// server tool constructors.
var anthropicServerTools = map[string]func() anthropic.ToolUnionParam{
	types.ServerToolWebSearch: func() anthropic.ToolUnionParam {
		return anthropic.ToolUnionParam{
			OfWebSearchTool20250305: &anthropic.WebSearchTool20250305Param{},
		}
	},
}

// convertTools converts types.ToolDefinition to anthropic tool params.
func convertTools(tools []types.ToolDefinition) ([]anthropic.ToolUnionParam, error) {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, tool := range tools {
		if tool.IsClientTool() {
			var schema map[string]interface{}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				return nil, fmt.Errorf("failed to parse tool schema: %w", err)
			}

			properties, _ := schema["properties"].(map[string]interface{})

			toolParam := anthropic.ToolParam{
				Name:        tool.Name,
				Description: param.NewOpt(tool.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: properties,
				},
			}
			result = append(result, anthropic.ToolUnionParam{OfTool: &toolParam})
			continue
		}

		// Server-side tool — skip if not supported by Anthropic.
		ctor, ok := anthropicServerTools[tool.Name]
		if !ok {
			continue
		}
		result = append(result, ctor())
	}

	return result, nil
}

// convertResponse converts anthropic response to types.CompletionResponse.
func convertResponse(resp *anthropic.Message) *types.CompletionResponse {
	content := make([]types.ContentBlock, 0, len(resp.Content))

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content = append(content, types.ContentBlock{
				Type: "text",
				Text: block.Text,
			})
		case "tool_use":
			content = append(content, types.ContentBlock{
				Type:  "tool_use",
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	return &types.CompletionResponse{
		ID:         resp.ID,
		Model:      string(resp.Model),
		Content:    content,
		StopReason: string(resp.StopReason),
		Usage: types.Usage{
			InputTokens:              int(resp.Usage.InputTokens),
			OutputTokens:             int(resp.Usage.OutputTokens),
			CacheReadInputTokens:     int(resp.Usage.CacheReadInputTokens),
			CacheCreationInputTokens: int(resp.Usage.CacheCreationInputTokens),
		},
	}
}

// processStreamEvents reads from an Anthropic SDK stream and converts events
// to types.StreamEvent, sending them on the provided channel.
func processStreamEvents(stream *ssestream.Stream[anthropic.MessageStreamEventUnion], events chan<- types.StreamEvent) {
	var currentToolUse *types.ContentBlock
	var currentText *types.ContentBlock
	var fullResponse *types.CompletionResponse

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "message_start":
			fullResponse = &types.CompletionResponse{
				ID:    event.Message.ID,
				Model: string(event.Message.Model),
				Usage: types.Usage{
					InputTokens:              int(event.Message.Usage.InputTokens),
					OutputTokens:             int(event.Message.Usage.OutputTokens),
					CacheReadInputTokens:     int(event.Message.Usage.CacheReadInputTokens),
					CacheCreationInputTokens: int(event.Message.Usage.CacheCreationInputTokens),
				},
			}
			events <- types.StreamEvent{Type: types.StreamEventStart}

		case "content_block_start":
			cb := event.ContentBlock
			switch cb.Type {
			case "tool_use":
				currentToolUse = &types.ContentBlock{
					Type: "tool_use",
					ID:   cb.ID,
					Name: cb.Name,
				}
			case "text":
				currentText = &types.ContentBlock{
					Type: "text",
				}
			}

		case "content_block_delta":
			delta := event.Delta
			switch delta.Type {
			case "text_delta":
				if currentText != nil {
					currentText.Text += delta.Text
				}
				events <- types.StreamEvent{
					Type:    types.StreamEventDelta,
					Content: delta.Text,
				}
			case "input_json_delta":
				if currentToolUse != nil {
					var existing string
					if currentToolUse.Input != nil {
						existing = string(currentToolUse.Input)
					}
					currentToolUse.Input = json.RawMessage(existing + delta.PartialJSON)
				}
			}

		case "content_block_stop":
			if currentText != nil {
				if fullResponse != nil {
					fullResponse.Content = append(fullResponse.Content, *currentText)
				}
				currentText = nil
			}
			if currentToolUse != nil {
				events <- types.StreamEvent{
					Type:         types.StreamEventContentDone,
					ContentBlock: currentToolUse,
				}
				if fullResponse != nil {
					fullResponse.Content = append(fullResponse.Content, *currentToolUse)
				}
				currentToolUse = nil
			}

		case "message_delta":
			if fullResponse != nil {
				fullResponse.StopReason = string(event.Delta.StopReason)
				fullResponse.Usage.OutputTokens = int(event.Usage.OutputTokens)
			}

		case "message_stop":
			if fullResponse != nil {
				events <- types.StreamEvent{
					Type:     types.StreamEventDone,
					Response: fullResponse,
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		events <- types.StreamEvent{
			Type:  types.StreamEventError,
			Error: err,
		}
	}
}
