package anthropic

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"langdag.com/langdag/types"
)

// BedrockProvider implements the provider interface for Anthropic via AWS Bedrock.
type BedrockProvider struct {
	client anthropic.Client
}

// NewBedrock creates a new Anthropic Bedrock provider.
// It uses AWS default credentials (env vars, shared config, IAM role, etc.).
func NewBedrock(ctx context.Context, region ...string) (*BedrockProvider, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if len(region) > 0 && region[0] != "" {
		opts = append(opts, awsconfig.WithRegion(region[0]))
	}
	client := anthropic.NewClient(
		bedrock.WithLoadDefaultConfig(ctx, opts...),
	)
	return &BedrockProvider{client: client}, nil
}

// Name returns the provider name.
func (p *BedrockProvider) Name() string {
	return "anthropic-bedrock"
}

// Models returns the available models.
func (p *BedrockProvider) Models() []types.ModelInfo {
	st := []string{types.ServerToolWebSearch}
	return []types.ModelInfo{
		{ID: "anthropic.claude-sonnet-4-20250514-v1:0", Name: "Claude Sonnet 4 (Bedrock)", ContextWindow: 200000, MaxOutput: 8192, ServerTools: st},
		{ID: "anthropic.claude-opus-4-20250514-v1:0", Name: "Claude Opus 4 (Bedrock)", ContextWindow: 200000, MaxOutput: 8192, ServerTools: st},
		{ID: "anthropic.claude-3-5-haiku-20241022-v1:0", Name: "Claude Haiku 3.5 (Bedrock)", ContextWindow: 200000, MaxOutput: 8192, ServerTools: st},
	}
}

// Complete performs a basic completion request.
func (p *BedrockProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	params, err := buildParams(req)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic-bedrock completion failed: %w", err)
	}

	return convertResponse(resp), nil
}

// Stream performs a streaming completion request.
func (p *BedrockProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
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
