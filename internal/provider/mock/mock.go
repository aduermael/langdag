// Package mock provides a mock implementation of the provider interface for testing.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"langdag.com/langdag/types"
)

// Config holds mock provider configuration.
type Config struct {
	Mode          string        // random, echo, fixed, tool_use, error, stream_error, partial_max_tokens
	FixedResponse string        // response text for fixed mode
	Delay         time.Duration // delay before responding
	ChunkDelay    time.Duration // delay between stream chunks
	// ToolCalls configures tool_use responses when Mode is "tool_use".
	// Each entry produces a tool_use content block in the response.
	ToolCalls []ToolCallConfig
	// StopReason overrides the default stop reason ("end_turn" or "tool_use").
	// When set with Mode "fixed" and empty FixedResponse, the response has no content.
	StopReason string
	// Error is returned by Complete() and Stream() when Mode is "error".
	// Also used as the error returned during the FailUntilCall transient-failure window.
	Error error
	// ErrorAfterChunks controls "stream_error" mode: Stream() sends this many
	// delta chunks before emitting a StreamEventError with Config.Error.
	ErrorAfterChunks int
	// FailUntilCall enables call-counting: calls 1..N return Config.Error,
	// call N+1 onwards use the normal mode. 0 disables (default).
	FailUntilCall int
}

// ToolCallConfig defines a mock tool call response.
type ToolCallConfig struct {
	Name  string          // Tool name
	Input json.RawMessage // Tool input as JSON
}

// Provider implements the provider interface with mock responses.
type Provider struct {
	cfg         Config
	LastRequest *types.CompletionRequest // captures the most recent request for testing
	callCount   int                      // tracks number of Complete/Stream calls for FailUntilCall
}

// New creates a new mock provider.
func New(cfg Config) *Provider {
	return &Provider{cfg: cfg}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "mock"
}

// Models returns the available mock models.
func (p *Provider) Models() []types.ModelInfo {
	return []types.ModelInfo{
		{ID: "mock-fast", Name: "Mock Fast", ContextWindow: 200000, MaxOutput: 8192},
		{ID: "mock-slow", Name: "Mock Slow", ContextWindow: 200000, MaxOutput: 8192},
	}
}

// shouldFail increments the call counter and returns true if the current call
// should return an error (either because Mode is "error" or because the call
// is within the FailUntilCall transient-failure window).
func (p *Provider) shouldFail() bool {
	p.callCount++
	if p.cfg.FailUntilCall > 0 && p.callCount <= p.cfg.FailUntilCall {
		return true
	}
	return p.cfg.Mode == "error"
}

// CallCount returns the number of Complete/Stream calls made so far.
func (p *Provider) CallCount() int {
	return p.callCount
}

// Complete performs a mock completion request.
func (p *Provider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	p.LastRequest = req
	if p.shouldFail() {
		return nil, p.cfg.Error
	}
	if p.cfg.Delay > 0 {
		select {
		case <-time.After(p.cfg.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	text := p.generateResponse(req)
	contentBlocks := p.generateContentBlocks(text)

	stopReason := p.resolveStopReason()

	inputTokens := estimateTokens(req)
	return &types.CompletionResponse{
		ID:         generateID(),
		Model:      req.Model,
		Content:    contentBlocks,
		StopReason: stopReason,
		Usage: types.Usage{
			InputTokens:              inputTokens,
			OutputTokens:             len(strings.Fields(text)),
			CacheReadInputTokens:     inputTokens / 4,
			CacheCreationInputTokens: inputTokens / 8,
			ReasoningTokens:          len(strings.Fields(text)) / 3,
		},
	}, nil
}

// Stream performs a mock streaming completion request.
func (p *Provider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	p.LastRequest = req
	if p.shouldFail() {
		return nil, p.cfg.Error
	}
	if p.cfg.Delay > 0 {
		select {
		case <-time.After(p.cfg.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	text := p.generateResponse(req)
	words := strings.Fields(text)
	events := make(chan types.StreamEvent, len(words)+len(p.cfg.ToolCalls)+5)

	go func() {
		defer close(events)

		// Start event
		events <- types.StreamEvent{Type: types.StreamEventStart}

		// stream_error mode: send ErrorAfterChunks deltas, then an error event
		if p.cfg.Mode == "stream_error" {
			for i := 0; i < p.cfg.ErrorAfterChunks && i < len(words); i++ {
				chunk := words[i]
				if i < len(words)-1 {
					chunk += " "
				}
				events <- types.StreamEvent{
					Type:    types.StreamEventDelta,
					Content: chunk,
				}
				if p.cfg.ChunkDelay > 0 {
					time.Sleep(p.cfg.ChunkDelay)
				}
			}
			events <- types.StreamEvent{Type: types.StreamEventError, Error: p.cfg.Error}
			return
		}

		// Delta events - word by word
		for i, word := range words {
			select {
			case <-ctx.Done():
				events <- types.StreamEvent{Type: types.StreamEventError, Error: ctx.Err()}
				return
			default:
			}

			chunk := word
			if i < len(words)-1 {
				chunk += " "
			}
			events <- types.StreamEvent{
				Type:    types.StreamEventDelta,
				Content: chunk,
			}

			if p.cfg.ChunkDelay > 0 {
				time.Sleep(p.cfg.ChunkDelay)
			}
		}

		// Emit content_done events for any tool_use blocks
		if p.cfg.Mode == "tool_use" {
			for i, tc := range p.cfg.ToolCalls {
				events <- types.StreamEvent{
					Type: types.StreamEventContentDone,
					ContentBlock: &types.ContentBlock{
						Type:  "tool_use",
						ID:    fmt.Sprintf("toolu_%06d", i),
						Name:  tc.Name,
						Input: tc.Input,
					},
				}
			}
		}

		contentBlocks := p.generateContentBlocks(text)
		stopReason := p.resolveStopReason()

		// Done event
		inputTokens := estimateTokens(req)
		events <- types.StreamEvent{
			Type: types.StreamEventDone,
			Response: &types.CompletionResponse{
				ID:         generateID(),
				Model:      req.Model,
				Content:    contentBlocks,
				StopReason: stopReason,
				Usage: types.Usage{
					InputTokens:              inputTokens,
					OutputTokens:             len(words),
					CacheReadInputTokens:     inputTokens / 4,
					CacheCreationInputTokens: inputTokens / 8,
					ReasoningTokens:          len(words) / 3,
				},
			},
		}
	}()

	return events, nil
}

func (p *Provider) generateResponse(req *types.CompletionRequest) string {
	switch p.cfg.Mode {
	case "echo":
		return echoLastMessage(req)
	case "fixed", "partial_max_tokens":
		if p.cfg.FixedResponse != "" {
			return p.cfg.FixedResponse
		}
		// When StopReason is explicitly set, respect empty FixedResponse literally
		// (e.g. simulating max_tokens with no content).
		if p.cfg.StopReason != "" || p.cfg.Mode == "partial_max_tokens" {
			return ""
		}
		return "This is a mock response."
	case "tool_use":
		// In tool_use mode, text is optional; tool calls come via content blocks.
		return p.cfg.FixedResponse
	case "stream_error":
		// stream_error uses FixedResponse to generate chunks before the error.
		if p.cfg.FixedResponse != "" {
			return p.cfg.FixedResponse
		}
		return "This is a mock response that will be interrupted."
	case "error":
		return ""
	default: // random
		return randomResponse()
	}
}

// resolveStopReason returns the stop reason for the response.
func (p *Provider) resolveStopReason() string {
	if p.cfg.StopReason != "" {
		return p.cfg.StopReason
	}
	if p.cfg.Mode == "partial_max_tokens" {
		return "max_tokens"
	}
	if p.cfg.Mode == "tool_use" && len(p.cfg.ToolCalls) > 0 {
		return "tool_use"
	}
	return "end_turn"
}

// generateContentBlocks returns the content blocks for a response.
// For tool_use mode this includes tool_use blocks alongside any text.
func (p *Provider) generateContentBlocks(text string) []types.ContentBlock {
	var blocks []types.ContentBlock
	if text != "" {
		blocks = append(blocks, types.ContentBlock{Type: "text", Text: text})
	}
	if p.cfg.Mode == "tool_use" {
		for i, tc := range p.cfg.ToolCalls {
			blocks = append(blocks, types.ContentBlock{
				Type:  "tool_use",
				ID:    fmt.Sprintf("toolu_%06d", i),
				Name:  tc.Name,
				Input: tc.Input,
			})
		}
	}
	if len(blocks) == 0 && p.cfg.StopReason == "" {
		blocks = append(blocks, types.ContentBlock{Type: "text", Text: text})
	}
	return blocks
}

func echoLastMessage(req *types.CompletionRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role == "user" {
			var s string
			if err := json.Unmarshal(msg.Content, &s); err == nil {
				return s
			}
			var blocks []types.ContentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err == nil {
				var parts []string
				for _, b := range blocks {
					if b.Type == "text" {
						parts = append(parts, b.Text)
					}
				}
				return strings.Join(parts, " ")
			}
			return string(msg.Content)
		}
	}
	return "No user message to echo."
}

func randomResponse() string {
	sentences := rand.Intn(5) + 3
	var result []string
	for i := 0; i < sentences; i++ {
		result = append(result, loremSentences[rand.Intn(len(loremSentences))])
	}
	return strings.Join(result, " ")
}

func estimateTokens(req *types.CompletionRequest) int {
	total := 0
	for _, msg := range req.Messages {
		var s string
		if err := json.Unmarshal(msg.Content, &s); err == nil {
			total += len(strings.Fields(s))
		}
	}
	if total == 0 {
		total = 10
	}
	return total
}

func generateID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 24)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("msg_%s", string(b))
}

var loremSentences = []string{
	"The quick brown fox jumps over the lazy dog.",
	"Lorem ipsum dolor sit amet, consectetur adipiscing elit.",
	"Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
	"Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.",
	"Duis aute irure dolor in reprehenderit in voluptate velit esse cillum.",
	"Excepteur sint occaecat cupidatat non proident, sunt in culpa.",
	"Nemo enim ipsam voluptatem quia voluptas sit aspernatur aut odit.",
	"Neque porro quisquam est, qui dolorem ipsum quia dolor sit amet.",
}
