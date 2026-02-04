// Package anthropic provides an Anthropic implementation of the provider interface.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/langdag/langdag/pkg/types"
)

// Provider implements the provider interface for Anthropic.
type Provider struct {
	client anthropic.Client
}

// New creates a new Anthropic provider.
func New(apiKey string) *Provider {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Provider{client: client}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "anthropic"
}

// Models returns the available models.
func (p *Provider) Models() []types.ModelInfo {
	return []types.ModelInfo{
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ContextWindow: 200000, MaxOutput: 8192},
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", ContextWindow: 200000, MaxOutput: 8192},
		{ID: "claude-haiku-3-5-20241022", Name: "Claude Haiku 3.5", ContextWindow: 200000, MaxOutput: 8192},
	}
}

// Complete performs a basic completion request.
func (p *Provider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	messages, err := convertMessages(req.Messages)
	if err != nil {
		return nil, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  messages,
		MaxTokens: int64(req.MaxTokens),
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.System},
		}
	}

	if req.Temperature > 0 {
		params.Temperature = param.NewOpt(req.Temperature)
	}

	if len(req.StopSeqs) > 0 {
		params.StopSequences = req.StopSeqs
	}

	if len(req.Tools) > 0 {
		tools, err := convertTools(req.Tools)
		if err != nil {
			return nil, err
		}
		params.Tools = tools
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic completion failed: %w", err)
	}

	return convertResponse(resp), nil
}

// Stream performs a streaming completion request.
func (p *Provider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	messages, err := convertMessages(req.Messages)
	if err != nil {
		return nil, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  messages,
		MaxTokens: int64(req.MaxTokens),
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.System},
		}
	}

	if req.Temperature > 0 {
		params.Temperature = param.NewOpt(req.Temperature)
	}

	if len(req.StopSeqs) > 0 {
		params.StopSequences = req.StopSeqs
	}

	if len(req.Tools) > 0 {
		tools, err := convertTools(req.Tools)
		if err != nil {
			return nil, err
		}
		params.Tools = tools
	}

	stream := p.client.Messages.NewStreaming(ctx, params)

	events := make(chan types.StreamEvent, 100)

	go func() {
		defer close(events)

		var currentToolUse *types.ContentBlock
		var fullResponse *types.CompletionResponse

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "message_start":
				fullResponse = &types.CompletionResponse{
					ID:    event.Message.ID,
					Model: string(event.Message.Model),
					Usage: types.Usage{
						InputTokens:  int(event.Message.Usage.InputTokens),
						OutputTokens: int(event.Message.Usage.OutputTokens),
					},
				}
				events <- types.StreamEvent{Type: types.StreamEventStart}

			case "content_block_start":
				cb := event.ContentBlock
				if cb.Type == "tool_use" {
					currentToolUse = &types.ContentBlock{
						Type: "tool_use",
						ID:   cb.ID,
						Name: cb.Name,
					}
				}

			case "content_block_delta":
				delta := event.Delta
				switch delta.Type {
				case "text_delta":
					events <- types.StreamEvent{
						Type:    types.StreamEventDelta,
						Content: delta.Text,
					}
				case "input_json_delta":
					if currentToolUse != nil {
						// Accumulate partial JSON
						var existing string
						if currentToolUse.Input != nil {
							existing = string(currentToolUse.Input)
						}
						// Delta.Text contains the partial JSON for input_json_delta
						currentToolUse.Input = json.RawMessage(existing + delta.Text)
					}
				}

			case "content_block_stop":
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
	}()

	return events, nil
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
					anthropicBlocks = append(anthropicBlocks, anthropic.NewTextBlock(block.Text))
				case "tool_result":
					anthropicBlocks = append(anthropicBlocks, anthropic.NewToolResultBlock(block.ToolUseID, block.Content, block.IsError))
				}
			}

			result = append(result, anthropic.MessageParam{
				Role:    anthropic.MessageParamRole(msg.Role),
				Content: anthropicBlocks,
			})
		} else {
			if msg.Role == "assistant" {
				result = append(result, anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
			} else {
				result = append(result, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
			}
		}
	}

	return result, nil
}

// convertTools converts types.ToolDefinition to anthropic tool params.
func convertTools(tools []types.ToolDefinition) ([]anthropic.ToolUnionParam, error) {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, tool := range tools {
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
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
		},
	}
}
