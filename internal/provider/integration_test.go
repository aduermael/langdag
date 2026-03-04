package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/langdag/langdag/pkg/types"
)

// configurableProvider allows fine-grained control over failure behavior for integration tests.
type configurableProvider struct {
	name          string
	failCount     int32 // number of calls that should fail (atomic)
	callCount     int32 // total calls made (atomic)
	failErr       error
	responseDelay time.Duration
}

func (p *configurableProvider) Name() string             { return p.name }
func (p *configurableProvider) Models() []types.ModelInfo { return []types.ModelInfo{{ID: p.name + "-model", Name: p.name}} }

func (p *configurableProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	n := atomic.AddInt32(&p.callCount, 1)
	if n <= atomic.LoadInt32(&p.failCount) {
		return nil, p.failErr
	}
	if p.responseDelay > 0 {
		select {
		case <-time.After(p.responseDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &types.CompletionResponse{
		ID:    fmt.Sprintf("resp-%s-%d", p.name, n),
		Model: req.Model,
		Content: []types.ContentBlock{
			{Type: "text", Text: "response from " + p.name},
		},
		StopReason: "end_turn",
		Usage:      types.Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

func (p *configurableProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	n := atomic.AddInt32(&p.callCount, 1)
	if n <= atomic.LoadInt32(&p.failCount) {
		return nil, p.failErr
	}
	ch := make(chan types.StreamEvent, 3)
	ch <- types.StreamEvent{Type: types.StreamEventStart}
	ch <- types.StreamEvent{Type: types.StreamEventDelta, Content: "response from " + p.name}
	ch <- types.StreamEvent{
		Type: types.StreamEventDone,
		Response: &types.CompletionResponse{
			ID:    fmt.Sprintf("resp-%s-%d", p.name, n),
			Model: req.Model,
			Content: []types.ContentBlock{
				{Type: "text", Text: "response from " + p.name},
			},
			StopReason: "end_turn",
			Usage:      types.Usage{InputTokens: 10, OutputTokens: 5},
		},
	}
	close(ch)
	return ch, nil
}

func integrationReq() *types.CompletionRequest {
	return &types.CompletionRequest{
		Model:     "test-model",
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}},
		MaxTokens: 100,
	}
}

// TestIntegration_RoutingWithRetryAndFallback tests the full stack:
// Router → Retry → Provider, with weighted selection and fallback.
func TestIntegration_RoutingWithRetryAndFallback(t *testing.T) {
	// Primary provider: fails twice with transient error, then succeeds.
	// With retry (max 2), it should succeed on the 3rd attempt.
	primary := &configurableProvider{
		name:      "primary",
		failCount: 2,
		failErr:   fmt.Errorf("status 503: service unavailable"),
	}
	fallback := &configurableProvider{name: "fallback"}

	retryCfg := RetryConfig{MaxRetries: 2, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	router, err := NewRouter(
		[]RouteEntry{{Provider: WithRetry(primary, retryCfg), Weight: 100}},
		[]Provider{WithRetry(fallback, retryCfg)},
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := router.Complete(context.Background(), integrationReq())
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if resp.Provider != "primary" {
		t.Errorf("expected provider 'primary', got %q", resp.Provider)
	}
	if atomic.LoadInt32(&primary.callCount) != 3 {
		t.Errorf("expected primary to be called 3 times, got %d", primary.callCount)
	}
	if atomic.LoadInt32(&fallback.callCount) != 0 {
		t.Errorf("expected fallback not to be called, got %d", fallback.callCount)
	}
}

// TestIntegration_RetryExhaustedThenFallback tests that when retries are
// exhausted on the primary, the router falls back to the next provider.
func TestIntegration_RetryExhaustedThenFallback(t *testing.T) {
	// Primary: fails 5 times (more than max retries)
	primary := &configurableProvider{
		name:      "primary",
		failCount: 5,
		failErr:   fmt.Errorf("status 500: internal server error"),
	}
	fallback := &configurableProvider{name: "fallback"}

	retryCfg := RetryConfig{MaxRetries: 2, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	router, err := NewRouter(
		[]RouteEntry{{Provider: WithRetry(primary, retryCfg), Weight: 100}},
		[]Provider{WithRetry(primary, retryCfg), WithRetry(fallback, retryCfg)},
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := router.Complete(context.Background(), integrationReq())
	if err != nil {
		t.Fatalf("expected fallback success, got: %v", err)
	}
	if resp.Provider != "fallback" {
		t.Errorf("expected provider 'fallback', got %q", resp.Provider)
	}
	// Primary: 3 calls from router attempt (1 + 2 retries), then skipped in fallback
	// Fallback: 1 call (succeeds first try)
	if atomic.LoadInt32(&fallback.callCount) != 1 {
		t.Errorf("expected fallback called once, got %d", fallback.callCount)
	}
}

// TestIntegration_NonTransientErrorSkipsRetry verifies that non-transient
// errors (e.g. 401) are not retried and immediately trigger fallback.
func TestIntegration_NonTransientErrorSkipsRetry(t *testing.T) {
	primary := &configurableProvider{
		name:      "primary",
		failCount: 100,
		failErr:   fmt.Errorf("status 401: unauthorized"),
	}
	fallback := &configurableProvider{name: "fallback"}

	retryCfg := RetryConfig{MaxRetries: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	router, err := NewRouter(
		[]RouteEntry{{Provider: WithRetry(primary, retryCfg), Weight: 100}},
		[]Provider{WithRetry(fallback, retryCfg)},
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := router.Complete(context.Background(), integrationReq())
	if err != nil {
		t.Fatalf("expected fallback success, got: %v", err)
	}
	if resp.Provider != "fallback" {
		t.Errorf("expected provider 'fallback', got %q", resp.Provider)
	}
	// Non-transient: primary should be called only once (no retries)
	if atomic.LoadInt32(&primary.callCount) != 1 {
		t.Errorf("expected primary called once (no retry for 401), got %d", primary.callCount)
	}
}

// TestIntegration_AllProvidersExhausted verifies error when every provider
// in both routing and fallback fails.
func TestIntegration_AllProvidersExhausted(t *testing.T) {
	p1 := &configurableProvider{name: "p1", failCount: 100, failErr: fmt.Errorf("status 503: unavailable")}
	p2 := &configurableProvider{name: "p2", failCount: 100, failErr: fmt.Errorf("status 503: unavailable")}

	retryCfg := RetryConfig{MaxRetries: 1, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	router, err := NewRouter(
		[]RouteEntry{{Provider: WithRetry(p1, retryCfg), Weight: 100}},
		[]Provider{WithRetry(p1, retryCfg), WithRetry(p2, retryCfg)},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = router.Complete(context.Background(), integrationReq())
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

// TestIntegration_StreamRoutingWithFallback tests the streaming path
// with retry and fallback.
func TestIntegration_StreamRoutingWithFallback(t *testing.T) {
	primary := &configurableProvider{
		name:      "primary",
		failCount: 5,
		failErr:   fmt.Errorf("status 502: bad gateway"),
	}
	fallback := &configurableProvider{name: "fallback"}

	retryCfg := RetryConfig{MaxRetries: 1, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	router, err := NewRouter(
		[]RouteEntry{{Provider: WithRetry(primary, retryCfg), Weight: 100}},
		[]Provider{WithRetry(fallback, retryCfg)},
	)
	if err != nil {
		t.Fatal(err)
	}

	ch, err := router.Stream(context.Background(), integrationReq())
	if err != nil {
		t.Fatalf("expected stream success via fallback, got: %v", err)
	}

	var events []types.StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// Expect start, delta, done
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	doneEvent := events[len(events)-1]
	if doneEvent.Type != types.StreamEventDone {
		t.Fatalf("expected done event, got %s", doneEvent.Type)
	}
	if doneEvent.Response.Provider != "fallback" {
		t.Errorf("expected provider 'fallback' in stream done, got %q", doneEvent.Response.Provider)
	}
}

// TestIntegration_WeightedRoutingWithPerProviderRetry tests that weighted
// routing distributes traffic and each provider has independent retry config.
func TestIntegration_WeightedRoutingWithPerProviderRetry(t *testing.T) {
	p1 := &configurableProvider{name: "p1"}
	p2 := &configurableProvider{name: "p2"}

	// Different retry configs per provider
	r1 := RetryConfig{MaxRetries: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	r2 := RetryConfig{MaxRetries: 1, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	router, err := NewRouter(
		[]RouteEntry{
			{Provider: WithRetry(p1, r1), Weight: 70},
			{Provider: WithRetry(p2, r2), Weight: 30},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	providerCounts := map[string]int{}
	for i := 0; i < 500; i++ {
		resp, err := router.Complete(context.Background(), integrationReq())
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
		providerCounts[resp.Provider]++
	}

	// With 70/30 weights over 500 calls, p1 should get ~350, p2 ~150.
	if providerCounts["p1"] < 250 || providerCounts["p1"] > 450 {
		t.Errorf("p1 expected ~350 calls, got %d", providerCounts["p1"])
	}
	if providerCounts["p2"] < 50 || providerCounts["p2"] > 250 {
		t.Errorf("p2 expected ~150 calls, got %d", providerCounts["p2"])
	}
}

// TestIntegration_ContextCancellation verifies that routing respects
// context cancellation during retry backoff.
func TestIntegration_ContextCancellation(t *testing.T) {
	primary := &configurableProvider{
		name:      "primary",
		failCount: 100,
		failErr:   fmt.Errorf("status 503: unavailable"),
	}

	retryCfg := RetryConfig{MaxRetries: 5, BaseDelay: 500 * time.Millisecond, MaxDelay: 2 * time.Second}

	router, err := NewRouter(
		[]RouteEntry{{Provider: WithRetry(primary, retryCfg), Weight: 100}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = router.Complete(ctx, integrationReq())
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestIntegration_FallbackChainOrder verifies the fallback chain is tried in order.
func TestIntegration_FallbackChainOrder(t *testing.T) {
	primary := &configurableProvider{name: "primary", failCount: 100, failErr: fmt.Errorf("status 500: error")}
	fb1 := &configurableProvider{name: "fb1", failCount: 100, failErr: fmt.Errorf("status 500: error")}
	fb2 := &configurableProvider{name: "fb2", failCount: 100, failErr: fmt.Errorf("status 500: error")}
	fb3 := &configurableProvider{name: "fb3"} // this one succeeds

	noRetry := RetryConfig{MaxRetries: 0}

	router, err := NewRouter(
		[]RouteEntry{{Provider: WithRetry(primary, noRetry), Weight: 100}},
		[]Provider{
			WithRetry(fb1, noRetry),
			WithRetry(fb2, noRetry),
			WithRetry(fb3, noRetry),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := router.Complete(context.Background(), integrationReq())
	if err != nil {
		t.Fatalf("expected fb3 to succeed, got: %v", err)
	}
	if resp.Provider != "fb3" {
		t.Errorf("expected provider 'fb3', got %q", resp.Provider)
	}

	// All providers before fb3 should have been called
	if atomic.LoadInt32(&fb1.callCount) != 1 {
		t.Errorf("fb1 should have been tried, calls: %d", fb1.callCount)
	}
	if atomic.LoadInt32(&fb2.callCount) != 1 {
		t.Errorf("fb2 should have been tried, calls: %d", fb2.callCount)
	}
	if atomic.LoadInt32(&fb3.callCount) != 1 {
		t.Errorf("fb3 should have been called once, calls: %d", fb3.callCount)
	}
}
