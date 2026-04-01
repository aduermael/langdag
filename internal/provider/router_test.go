package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"langdag.com/langdag/types"
)

// testProvider is a minimal provider for testing.
type testProvider struct {
	name     string
	failNext bool
	models   []types.ModelInfo
	calls    int
}

func (p *testProvider) Name() string { return p.name }
func (p *testProvider) Models() []types.ModelInfo {
	if p.models != nil {
		return p.models
	}
	return []types.ModelInfo{{ID: "test-model", Name: "Test"}}
}

func (p *testProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	p.calls++
	if p.failNext {
		return nil, fmt.Errorf("%s: intentional failure", p.name)
	}
	return &types.CompletionResponse{
		ID:    "resp-" + p.name,
		Model: req.Model,
		Content: []types.ContentBlock{
			{Type: "text", Text: "response from " + p.name},
		},
	}, nil
}

func (p *testProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	p.calls++
	if p.failNext {
		return nil, fmt.Errorf("%s: intentional failure", p.name)
	}
	ch := make(chan types.StreamEvent, 2)
	ch <- types.StreamEvent{Type: types.StreamEventStart}
	ch <- types.StreamEvent{
		Type:     types.StreamEventDone,
		Response: &types.CompletionResponse{ID: "resp-" + p.name, Model: req.Model},
	}
	close(ch)
	return ch, nil
}

func testReq() *types.CompletionRequest {
	return &types.CompletionRequest{
		Model:     "test-model",
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}},
		MaxTokens: 100,
	}
}

func TestRouterSingleProvider(t *testing.T) {
	p := &testProvider{name: "p1"}
	r, err := NewRouter(
		[]RouteEntry{{Provider: p, Weight: 100}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := r.Complete(context.Background(), testReq())
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "resp-p1" {
		t.Errorf("expected resp-p1, got %s", resp.ID)
	}
	if resp.Provider != "p1" {
		t.Errorf("expected provider p1, got %s", resp.Provider)
	}
}

func TestRouterWeightedDistribution(t *testing.T) {
	p1 := &testProvider{name: "p1"}
	p2 := &testProvider{name: "p2"}
	r, _ := NewRouter(
		[]RouteEntry{
			{Provider: p1, Weight: 90},
			{Provider: p2, Weight: 10},
		},
		nil,
	)

	for i := 0; i < 1000; i++ {
		r.Complete(context.Background(), testReq())
	}

	// p1 should get ~900 calls, p2 ~100. Allow wide tolerance.
	if p1.calls < 700 || p1.calls > 990 {
		t.Errorf("p1 expected ~900 calls, got %d", p1.calls)
	}
	if p2.calls < 10 || p2.calls > 300 {
		t.Errorf("p2 expected ~100 calls, got %d", p2.calls)
	}
}

func TestRouterFallbackOnError(t *testing.T) {
	primary := &testProvider{name: "primary", failNext: true}
	fallback := &testProvider{name: "fallback"}
	r, _ := NewRouter(
		[]RouteEntry{{Provider: primary, Weight: 100}},
		[]Provider{primary, fallback},
	)

	resp, err := r.Complete(context.Background(), testReq())
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "resp-fallback" {
		t.Errorf("expected resp-fallback, got %s", resp.ID)
	}
	if resp.Provider != "fallback" {
		t.Errorf("expected provider fallback, got %s", resp.Provider)
	}
}

func TestRouterAllFail(t *testing.T) {
	p1 := &testProvider{name: "p1", failNext: true}
	p2 := &testProvider{name: "p2", failNext: true}
	r, _ := NewRouter(
		[]RouteEntry{{Provider: p1, Weight: 100}},
		[]Provider{p1, p2},
	)

	_, err := r.Complete(context.Background(), testReq())
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestRouterStreamFallback(t *testing.T) {
	primary := &testProvider{name: "primary", failNext: true}
	fallback := &testProvider{name: "fallback"}
	r, _ := NewRouter(
		[]RouteEntry{{Provider: primary, Weight: 100}},
		[]Provider{primary, fallback},
	)

	ch, err := r.Stream(context.Background(), testReq())
	if err != nil {
		t.Fatal(err)
	}

	var events []types.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	doneEvent := events[len(events)-1]
	if doneEvent.Response.ID != "resp-fallback" {
		t.Error("expected fallback response")
	}
	if doneEvent.Response.Provider != "fallback" {
		t.Errorf("expected provider fallback, got %s", doneEvent.Response.Provider)
	}
}

func TestRouterNoProviders(t *testing.T) {
	_, err := NewRouter(nil, nil)
	if err == nil {
		t.Fatal("expected error with no providers")
	}
}

func TestRouterName(t *testing.T) {
	p := &testProvider{name: "p1"}
	r, _ := NewRouter([]RouteEntry{{Provider: p, Weight: 100}}, nil)
	if r.Name() != "router" {
		t.Errorf("expected name 'router', got '%s'", r.Name())
	}
}

func TestRouterModels(t *testing.T) {
	p1 := &testProvider{name: "p1", models: []types.ModelInfo{{ID: "m1", Name: "M1"}}}
	p2 := &testProvider{name: "p2", models: []types.ModelInfo{{ID: "m1", Name: "M1"}, {ID: "m2", Name: "M2"}}}
	r, _ := NewRouter(
		[]RouteEntry{{Provider: p1, Weight: 50}, {Provider: p2, Weight: 50}},
		nil,
	)

	models := r.Models()
	if len(models) != 2 {
		t.Errorf("expected 2 unique models, got %d", len(models))
	}
}

func TestRouterFallbackOnly(t *testing.T) {
	// No weighted entries, only fallback
	fb := &testProvider{name: "fb"}
	r, _ := NewRouter(nil, []Provider{fb})

	resp, err := r.Complete(context.Background(), testReq())
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "resp-fb" {
		t.Errorf("expected resp-fb, got %s", resp.ID)
	}
}

// TestRouterConcurrentComplete verifies no data races when calling Complete
// from multiple goroutines simultaneously. Run with -race.
func TestRouterConcurrentComplete(t *testing.T) {
	p1 := &configurableProvider{name: "p1"}
	p2 := &configurableProvider{name: "p2"}
	fb := &configurableProvider{name: "fb"}
	r, err := NewRouter(
		[]RouteEntry{
			{Provider: p1, Weight: 50},
			{Provider: p2, Weight: 50},
		},
		[]Provider{fb},
	)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := r.Complete(context.Background(), integrationReq())
			if err != nil {
				errs <- err
				return
			}
			if resp.Provider != "p1" && resp.Provider != "p2" {
				errs <- fmt.Errorf("unexpected provider: %s", resp.Provider)
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestRouterConcurrentStream verifies no data races when calling Stream
// from multiple goroutines simultaneously. Run with -race.
func TestRouterConcurrentStream(t *testing.T) {
	p1 := &configurableProvider{name: "p1"}
	p2 := &configurableProvider{name: "p2"}
	fb := &configurableProvider{name: "fb"}
	r, err := NewRouter(
		[]RouteEntry{
			{Provider: p1, Weight: 50},
			{Provider: p2, Weight: 50},
		},
		[]Provider{fb},
	)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, err := r.Stream(context.Background(), integrationReq())
			if err != nil {
				errs <- err
				return
			}
			var doneEvent types.StreamEvent
			for e := range ch {
				if e.Type == types.StreamEventDone {
					doneEvent = e
				}
			}
			if doneEvent.Response == nil {
				errs <- fmt.Errorf("missing done response")
				return
			}
			if doneEvent.Response.Provider != "p1" && doneEvent.Response.Provider != "p2" {
				errs <- fmt.Errorf("unexpected provider: %s", doneEvent.Response.Provider)
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestRouterConcurrentWithFailures verifies no races when some providers fail
// and fallback is exercised concurrently.
func TestRouterConcurrentWithFailures(t *testing.T) {
	// Primary fails half the time (odd calls fail via high failCount + atomic)
	primary := &configurableProvider{
		name:      "primary",
		failCount: 25, // first 25 calls fail
		failErr:   fmt.Errorf("primary: intentional failure"),
	}
	fallback := &configurableProvider{name: "fallback"}
	r, err := NewRouter(
		[]RouteEntry{{Provider: primary, Weight: 100}},
		[]Provider{fallback},
	)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := r.Complete(context.Background(), integrationReq())
			if err != nil {
				errs <- err
				return
			}
			if resp.Provider != "primary" && resp.Provider != "fallback" {
				errs <- fmt.Errorf("unexpected provider: %s", resp.Provider)
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestRouterFallbackContextCanceled verifies that when context is canceled,
// the router still tries fallbacks but they also fail due to canceled context.
func TestRouterFallbackContextCanceled(t *testing.T) {
	// Primary always fails.
	primary := &configurableProvider{
		name:      "primary",
		failCount: 100,
		failErr:   fmt.Errorf("status 503: unavailable"),
	}
	// Fallback has a response delay, so it checks ctx.Done().
	fallback := &configurableProvider{
		name:          "fallback",
		responseDelay: 5 * time.Second,
	}

	r, _ := NewRouter(
		[]RouteEntry{{Provider: primary, Weight: 100}},
		[]Provider{fallback},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	start := time.Now()
	_, err := r.Complete(ctx, integrationReq())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error with canceled context")
	}
	// Should fail fast — fallback sees canceled context and exits immediately.
	if elapsed > 500*time.Millisecond {
		t.Errorf("took %v — should fail fast with canceled context", elapsed)
	}
}

// TestRouterFallbackTransientError verifies that transient errors from the
// primary trigger a successful fallback (context remains valid).
func TestRouterFallbackTransientError(t *testing.T) {
	primary := &configurableProvider{
		name:      "primary",
		failCount: 100,
		failErr:   fmt.Errorf("status 503: unavailable"),
	}
	fallback := &configurableProvider{name: "fallback"}

	r, _ := NewRouter(
		[]RouteEntry{{Provider: primary, Weight: 100}},
		[]Provider{fallback},
	)

	resp, err := r.Complete(context.Background(), integrationReq())
	if err != nil {
		t.Fatalf("expected fallback success, got: %v", err)
	}
	if resp.Provider != "fallback" {
		t.Errorf("expected provider 'fallback', got %q", resp.Provider)
	}
}

// TestRouterStreamConcurrentWithFallback verifies no races on the streaming
// path when fallbacks are exercised concurrently.
func TestRouterStreamConcurrentWithFallback(t *testing.T) {
	primary := &configurableProvider{
		name:      "primary",
		failCount: 25,
		failErr:   fmt.Errorf("primary: stream failure"),
	}
	fallback := &configurableProvider{name: "fallback"}
	r, err := NewRouter(
		[]RouteEntry{{Provider: primary, Weight: 100}},
		[]Provider{fallback},
	)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, err := r.Stream(context.Background(), integrationReq())
			if err != nil {
				errs <- err
				return
			}
			for range ch {
				// drain
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
