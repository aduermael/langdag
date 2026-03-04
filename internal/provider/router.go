package provider

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/langdag/langdag/pkg/types"
)

// RouteEntry associates a provider with a routing weight.
type RouteEntry struct {
	Provider Provider
	Weight   int // relative weight for random selection
}

// Router implements the Provider interface by routing requests across
// multiple providers using weighted random selection and a fallback chain.
type Router struct {
	entries       []RouteEntry
	fallbackOrder []Provider
	totalWeight   int
}

// NewRouter creates a Router from the given route entries and fallback order.
// Entries with weight 0 are excluded from weighted selection but may still
// appear in the fallback chain.
func NewRouter(entries []RouteEntry, fallbackOrder []Provider) (*Router, error) {
	if len(entries) == 0 && len(fallbackOrder) == 0 {
		return nil, fmt.Errorf("router: at least one provider must be configured")
	}

	total := 0
	for _, e := range entries {
		total += e.Weight
	}

	return &Router{
		entries:       entries,
		fallbackOrder: fallbackOrder,
		totalWeight:   total,
	}, nil
}

// Name returns "router".
func (r *Router) Name() string {
	return "router"
}

// Models returns the union of all provider models.
func (r *Router) Models() []types.ModelInfo {
	seen := map[string]bool{}
	var models []types.ModelInfo
	for _, e := range r.entries {
		for _, m := range e.Provider.Models() {
			if !seen[m.ID] {
				seen[m.ID] = true
				models = append(models, m)
			}
		}
	}
	for _, p := range r.fallbackOrder {
		for _, m := range p.Models() {
			if !seen[m.ID] {
				seen[m.ID] = true
				models = append(models, m)
			}
		}
	}
	return models
}

// Complete routes the request to a weighted-random provider, falling back
// through the fallback chain on failure.
func (r *Router) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	primary := r.selectProvider()
	if primary != nil {
		resp, err := primary.Complete(ctx, req)
		if err == nil {
			resp.Provider = primary.Name()
			return resp, nil
		}
		// Primary failed, try fallback chain (skipping the primary)
		return r.completeFallback(ctx, req, primary, err)
	}
	// No weighted entries, go straight to fallback
	return r.completeFallback(ctx, req, nil, fmt.Errorf("router: no weighted providers available"))
}

// Stream routes the request to a weighted-random provider, falling back
// through the fallback chain on failure.
func (r *Router) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	primary := r.selectProvider()
	if primary != nil {
		ch, err := primary.Stream(ctx, req)
		if err == nil {
			return tagStreamProvider(ch, primary.Name()), nil
		}
		return r.streamFallback(ctx, req, primary, err)
	}
	return r.streamFallback(ctx, req, nil, fmt.Errorf("router: no weighted providers available"))
}

// selectProvider picks a provider based on weighted random selection.
// Returns nil if there are no weighted entries.
func (r *Router) selectProvider() Provider {
	if r.totalWeight == 0 || len(r.entries) == 0 {
		return nil
	}
	if len(r.entries) == 1 {
		return r.entries[0].Provider
	}

	n := rand.Intn(r.totalWeight)
	for _, e := range r.entries {
		n -= e.Weight
		if n < 0 {
			return e.Provider
		}
	}
	return r.entries[len(r.entries)-1].Provider
}

func (r *Router) completeFallback(ctx context.Context, req *types.CompletionRequest, skip Provider, lastErr error) (*types.CompletionResponse, error) {
	for _, p := range r.fallbackOrder {
		if skip != nil && p.Name() == skip.Name() {
			continue
		}
		resp, err := p.Complete(ctx, req)
		if err == nil {
			resp.Provider = p.Name()
			return resp, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("router: all providers failed, last error: %w", lastErr)
}

func (r *Router) streamFallback(ctx context.Context, req *types.CompletionRequest, skip Provider, lastErr error) (<-chan types.StreamEvent, error) {
	for _, p := range r.fallbackOrder {
		if skip != nil && p.Name() == skip.Name() {
			continue
		}
		ch, err := p.Stream(ctx, req)
		if err == nil {
			return tagStreamProvider(ch, p.Name()), nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("router: all providers failed, last error: %w", lastErr)
}

// tagStreamProvider wraps a stream channel to set the Provider field on the
// done event's CompletionResponse.
func tagStreamProvider(ch <-chan types.StreamEvent, providerName string) <-chan types.StreamEvent {
	out := make(chan types.StreamEvent, cap(ch))
	go func() {
		defer close(out)
		for event := range ch {
			if event.Type == types.StreamEventDone && event.Response != nil {
				event.Response.Provider = providerName
			}
			out <- event
		}
	}()
	return out
}
