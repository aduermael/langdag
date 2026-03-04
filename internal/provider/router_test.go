package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/langdag/langdag/pkg/types"
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
