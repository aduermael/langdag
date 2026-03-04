package anthropic

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/langdag/langdag/pkg/types"
)

// Provider implements the provider interface for the direct Anthropic API.
type Provider struct {
	client anthropic.Client
}

// New creates a new direct Anthropic provider.
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
	params, err := buildParams(req)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic completion failed: %w", err)
	}

	return convertResponse(resp), nil
}

// Stream performs a streaming completion request.
func (p *Provider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	params, err := buildParams(req)
	if err != nil {
		return nil, err
	}

	stream := p.client.Messages.NewStreaming(ctx, params)

	events := make(chan types.StreamEvent, 100)

	go func() {
		defer close(events)
		processStreamEvents(stream, events)
	}()

	return events, nil
}
