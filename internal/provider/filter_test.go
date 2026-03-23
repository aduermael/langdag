package provider

import (
	"context"
	"encoding/json"
	"testing"

	"langdag.com/langdag/types"
)

// stubProvider is a minimal Provider for testing the filter wrapper.
type stubProvider struct {
	models []types.ModelInfo
	// lastReq captures the request passed to Complete for verification.
	lastReq *types.CompletionRequest
}

func (s *stubProvider) Name() string              { return "stub" }
func (s *stubProvider) Models() []types.ModelInfo  { return s.models }

func (s *stubProvider) Complete(_ context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	s.lastReq = req
	return &types.CompletionResponse{}, nil
}

func (s *stubProvider) Stream(_ context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	s.lastReq = req
	ch := make(chan types.StreamEvent)
	close(ch)
	return ch, nil
}

func clientTool(name string) types.ToolDefinition {
	return types.ToolDefinition{
		Name:        name,
		Description: name,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func serverTool(name string) types.ToolDefinition {
	return types.ToolDefinition{
		Name:        name,
		Description: name,
	}
}

func TestFilterProvider_AllSupported(t *testing.T) {
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "model-a", ServerTools: []string{"web_search"}},
		},
	}
	fp := WithServerToolFilter(inner)

	req := &types.CompletionRequest{
		Model: "model-a",
		Tools: []types.ToolDefinition{serverTool("web_search"), clientTool("my_func")},
	}
	fp.Complete(context.Background(), req)

	if len(inner.lastReq.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(inner.lastReq.Tools))
	}
}

func TestFilterProvider_UnsupportedStripped(t *testing.T) {
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "model-a", ServerTools: []string{"web_search"}},
		},
	}
	fp := WithServerToolFilter(inner)

	req := &types.CompletionRequest{
		Model: "model-a",
		Tools: []types.ToolDefinition{
			serverTool("web_search"),
			serverTool("code_interpreter"), // unsupported
			clientTool("my_func"),
		},
	}
	fp.Complete(context.Background(), req)

	if len(inner.lastReq.Tools) != 2 {
		t.Errorf("expected 2 tools (web_search + my_func), got %d", len(inner.lastReq.Tools))
	}
	for _, tool := range inner.lastReq.Tools {
		if tool.Name == "code_interpreter" {
			t.Error("code_interpreter should have been stripped")
		}
	}
}

func TestFilterProvider_NoServerToolsSupported(t *testing.T) {
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "ollama-model"}, // no ServerTools
		},
	}
	fp := WithServerToolFilter(inner)

	req := &types.CompletionRequest{
		Model: "ollama-model",
		Tools: []types.ToolDefinition{
			serverTool("web_search"),
			clientTool("my_func"),
		},
	}
	fp.Complete(context.Background(), req)

	if len(inner.lastReq.Tools) != 1 {
		t.Errorf("expected 1 tool (my_func only), got %d", len(inner.lastReq.Tools))
	}
	if inner.lastReq.Tools[0].Name != "my_func" {
		t.Errorf("expected my_func, got %s", inner.lastReq.Tools[0].Name)
	}
}

func TestFilterProvider_NoToolsInRequest(t *testing.T) {
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "model-a", ServerTools: []string{"web_search"}},
		},
	}
	fp := WithServerToolFilter(inner)

	req := &types.CompletionRequest{Model: "model-a"}
	fp.Complete(context.Background(), req)

	if len(inner.lastReq.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(inner.lastReq.Tools))
	}
}

func TestFilterProvider_UnknownModelUsesProviderFallback(t *testing.T) {
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "model-a", ServerTools: []string{"web_search"}},
		},
	}
	fp := WithServerToolFilter(inner)

	// Unknown model — falls back to provider-level capabilities (union of
	// all known models' server tools), so web_search is preserved.
	req := &types.CompletionRequest{
		Model: "unknown-model",
		Tools: []types.ToolDefinition{
			serverTool("web_search"),
			clientTool("my_func"),
		},
	}
	fp.Complete(context.Background(), req)

	if len(inner.lastReq.Tools) != 2 {
		t.Errorf("expected 2 tools (web_search + my_func), got %d", len(inner.lastReq.Tools))
	}
}

func TestFilterProvider_CatalogModelIDFallback(t *testing.T) {
	// Simulates the real bug: Provider.Models() uses IDs like
	// "claude-sonnet-4-20250514" but callers use catalog IDs like
	// "claude-sonnet-4-6". The filter should fall back to provider-level
	// capabilities for unrecognized IDs.
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "claude-sonnet-4-20250514", ServerTools: []string{"web_search"}},
			{ID: "claude-opus-4-20250514", ServerTools: []string{"web_search"}},
		},
	}
	fp := WithServerToolFilter(inner)

	req := &types.CompletionRequest{
		Model: "claude-sonnet-4-6", // catalog ID, not in Models()
		Tools: []types.ToolDefinition{
			serverTool("web_search"),
			clientTool("my_func"),
		},
	}
	fp.Complete(context.Background(), req)

	if len(inner.lastReq.Tools) != 2 {
		t.Errorf("expected 2 tools (web_search + my_func), got %d", len(inner.lastReq.Tools))
	}
	for _, tool := range inner.lastReq.Tools {
		if tool.Name != "web_search" && tool.Name != "my_func" {
			t.Errorf("unexpected tool: %s", tool.Name)
		}
	}
}

func TestFilterProvider_PerModelCapabilities(t *testing.T) {
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "model-with-search", ServerTools: []string{"web_search"}},
			{ID: "model-without-search"}, // no server tools
		},
	}
	fp := WithServerToolFilter(inner)

	tools := []types.ToolDefinition{serverTool("web_search"), clientTool("my_func")}

	// Model with search: web_search preserved
	req1 := &types.CompletionRequest{Model: "model-with-search", Tools: tools}
	fp.Complete(context.Background(), req1)
	if len(inner.lastReq.Tools) != 2 {
		t.Errorf("model-with-search: expected 2 tools, got %d", len(inner.lastReq.Tools))
	}

	// Model without search: web_search stripped
	req2 := &types.CompletionRequest{Model: "model-without-search", Tools: tools}
	fp.Complete(context.Background(), req2)
	if len(inner.lastReq.Tools) != 1 {
		t.Errorf("model-without-search: expected 1 tool, got %d", len(inner.lastReq.Tools))
	}
}

func TestFilterProvider_StreamFilters(t *testing.T) {
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "model-a"}, // no server tools
		},
	}
	fp := WithServerToolFilter(inner)

	req := &types.CompletionRequest{
		Model: "model-a",
		Tools: []types.ToolDefinition{serverTool("web_search")},
	}
	fp.Stream(context.Background(), req)

	if len(inner.lastReq.Tools) != 0 {
		t.Errorf("Stream: expected 0 tools, got %d", len(inner.lastReq.Tools))
	}
}

func TestFilterProvider_PassthroughMethods(t *testing.T) {
	inner := &stubProvider{
		models: []types.ModelInfo{
			{ID: "m1", ServerTools: []string{"web_search"}},
		},
	}
	fp := WithServerToolFilter(inner)

	if fp.Name() != "stub" {
		t.Errorf("Name() = %q, want %q", fp.Name(), "stub")
	}

	models := fp.Models()
	if len(models) != 1 || models[0].ID != "m1" {
		t.Errorf("Models() = %v, want [{ID:m1}]", models)
	}
}
