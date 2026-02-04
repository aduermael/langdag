// Package provider defines the provider interface for LLM providers.
package provider

import (
	"context"

	"github.com/langdag/langdag/pkg/types"
)

// Provider defines the interface for LLM providers.
type Provider interface {
	// Complete performs a basic completion request.
	Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error)

	// Stream performs a streaming completion request.
	Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error)

	// Name returns the provider name.
	Name() string

	// Models returns the available models.
	Models() []types.ModelInfo
}
