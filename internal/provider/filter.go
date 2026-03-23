package provider

import (
	"context"

	"langdag.com/langdag/types"
)

// filterProvider wraps a Provider and silently removes server tools that
// the target model does not support, based on ModelInfo.ServerTools.
type filterProvider struct {
	inner Provider
	// modelTools maps model ID → set of supported server tool names.
	modelTools map[string]map[string]bool
	// providerFallback is the union of all server tools across the
	// provider's known models. Used for models not in modelTools (e.g.
	// catalog model IDs that differ from the hardcoded Models() list).
	providerFallback map[string]bool
}

// WithServerToolFilter wraps a Provider so that unsupported server tools are
// silently stripped from CompletionRequests before they reach the inner provider.
// Capability data comes from the inner provider's Models() method.
//
// Models explicitly listed by the provider use their declared ServerTools.
// Models not in the list (e.g. catalog IDs) fall back to the union of all
// server tools the provider's known models support.
func WithServerToolFilter(p Provider) Provider {
	modelTools := map[string]map[string]bool{}
	providerFallback := map[string]bool{}
	for _, m := range p.Models() {
		set := make(map[string]bool, len(m.ServerTools))
		for _, st := range m.ServerTools {
			set[st] = true
			providerFallback[st] = true
		}
		modelTools[m.ID] = set
	}
	return &filterProvider{inner: p, modelTools: modelTools, providerFallback: providerFallback}
}

func (f *filterProvider) Name() string              { return f.inner.Name() }
func (f *filterProvider) Models() []types.ModelInfo  { return f.inner.Models() }

func (f *filterProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	return f.inner.Complete(ctx, f.filterTools(req))
}

func (f *filterProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	return f.inner.Stream(ctx, f.filterTools(req))
}

// filterTools returns a (possibly modified) copy of req with unsupported
// server tools removed. Client tools are always preserved.
func (f *filterProvider) filterTools(req *types.CompletionRequest) *types.CompletionRequest {
	if len(req.Tools) == 0 {
		return req
	}

	supported, known := f.modelTools[req.Model]
	if !known {
		supported = f.providerFallback
	}

	// Fast path: check if any filtering is needed.
	needsFilter := false
	for _, t := range req.Tools {
		if !t.IsClientTool() && !supported[t.Name] {
			needsFilter = true
			break
		}
	}
	if !needsFilter {
		return req
	}

	// Shallow-copy the request and rebuild the tools slice.
	filtered := *req
	filtered.Tools = make([]types.ToolDefinition, 0, len(req.Tools))
	for _, t := range req.Tools {
		if t.IsClientTool() || supported[t.Name] {
			filtered.Tools = append(filtered.Tools, t)
		}
	}
	return &filtered
}
