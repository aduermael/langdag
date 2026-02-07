// Package mock provides a mock implementation of the provider interface for testing.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/langdag/langdag/pkg/types"
)

// Config holds mock provider configuration.
type Config struct {
	Mode          string        // random, echo, fixed
	FixedResponse string        // response text for fixed mode
	Delay         time.Duration // delay before responding
	ChunkDelay    time.Duration // delay between stream chunks
}

// Provider implements the provider interface with mock responses.
type Provider struct {
	cfg Config
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

// Complete performs a mock completion request.
func (p *Provider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	if p.cfg.Delay > 0 {
		select {
		case <-time.After(p.cfg.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	text := p.generateResponse(req)

	return &types.CompletionResponse{
		ID:    generateID(),
		Model: req.Model,
		Content: []types.ContentBlock{
			{Type: "text", Text: text},
		},
		StopReason: "end_turn",
		Usage: types.Usage{
			InputTokens:  estimateTokens(req),
			OutputTokens: len(strings.Fields(text)),
		},
	}, nil
}

// Stream performs a mock streaming completion request.
func (p *Provider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	if p.cfg.Delay > 0 {
		select {
		case <-time.After(p.cfg.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	text := p.generateResponse(req)
	words := strings.Fields(text)
	events := make(chan types.StreamEvent, len(words)+3)

	go func() {
		defer close(events)

		// Start event
		events <- types.StreamEvent{Type: types.StreamEventStart}

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

		// Done event
		events <- types.StreamEvent{
			Type: types.StreamEventDone,
			Response: &types.CompletionResponse{
				ID:    generateID(),
				Model: req.Model,
				Content: []types.ContentBlock{
					{Type: "text", Text: text},
				},
				StopReason: "end_turn",
				Usage: types.Usage{
					InputTokens:  estimateTokens(req),
					OutputTokens: len(words),
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
	case "fixed":
		if p.cfg.FixedResponse != "" {
			return p.cfg.FixedResponse
		}
		return "This is a mock response."
	default: // random
		return randomResponse()
	}
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
