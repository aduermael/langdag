package openai

import (
"context"
"encoding/json"
"fmt"
"net/http"
"net/http/httptest"
"testing"

"langdag.com/langdag/types"
)

// --- Models() ---

func TestOllamaModels_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.URL.Path == "/api/tags" {
w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": []}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if models == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

func TestOllamaModels_SingleWithContextWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.URL.Path == "/api/tags" {
w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "llama3"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"model_info": {"llama.context_length": 8192}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "llama3" || models[0].Name != "llama3" {
		t.Errorf("unexpected model: %+v", models[0])
	}
	if models[0].ContextWindow != 8192 {
		t.Errorf("expected context window 8192, got %d", models[0].ContextWindow)
	}
}

func TestOllamaModels_ParallelFetch(t *testing.T) {
	modelCtx := map[string]int{
		"model-a": 4096,
		"model-b": 8192,
		"model-c": 32768,
		"model-d": 131072,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.URL.Path == "/api/tags" {
w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models":[{"name":"model-a"},{"name":"model-b"},{"name":"model-c"},{"name":"model-d"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"model_info":{"llama.context_length":%d}}`, modelCtx[req["name"]])))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if len(models) != 4 {
		t.Fatalf("expected 4 models, got %d", len(models))
	}
	got := map[string]int{}
	for _, m := range models {
		got[m.ID] = m.ContextWindow
	}
	for name, expected := range modelCtx {
		if got[name] != expected {
			t.Errorf("%s: expected %d, got %d", name, expected, got[name])
		}
	}
}

func TestOllamaModels_TagsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusInternalServerError)
}))
	defer server.Close()

	p := NewOllama(server.URL)
	if models := p.Models(); models != nil {
		t.Errorf("expected nil on error, got %v", models)
	}
}

// --- Context window discovery ---

func TestOllamaContextWindow_NonStandardPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.URL.Path == "/api/tags" {
w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "qwen2.5:7b"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"model_info": {"qwen2.context_length": 32768}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ContextWindow != 32768 {
		t.Errorf("expected 32768, got %d", models[0].ContextWindow)
	}
}

func TestOllamaContextWindow_MissingKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.URL.Path == "/api/tags" {
w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "unknown-model"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"model_info": {}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ContextWindow != 0 {
		t.Errorf("expected 0, got %d", models[0].ContextWindow)
	}
}

func TestOllamaContextWindow_ShowError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.URL.Path == "/api/tags" {
w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "llama3"}]}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ContextWindow != 0 {
		t.Errorf("expected 0 on /api/show error, got %d", models[0].ContextWindow)
	}
}

func TestOllamaContextWindow_PerModelValues(t *testing.T) {
	modelCtx := map[string]int{"test-model-a": 8192, "test-model-b": 32768}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.URL.Path == "/api/tags" {
w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "test-model-a"}, {"name": "test-model-b"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"model_info": {"llama.context_length": %d}}`, modelCtx[req["name"]])))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	got := map[string]int{}
	for _, m := range models {
		got[m.ID] = m.ContextWindow
	}
	for name, expected := range modelCtx {
		if got[name] != expected {
			t.Errorf("%s: expected %d, got %d", name, expected, got[name])
		}
	}
}

func TestOllamaContextWindow_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if r.URL.Path == "/api/tags" {
w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "llama3"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"model_info": {"llama.context_length": 8192}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	p.Models()
	p.Models()

	if callCount != 1 {
		t.Errorf("expected /api/show called once, got %d", callCount)
	}
}

// --- Complete() ---

func TestOllamaComplete_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"resp-1","model":"llama3","choices":[{"message":{"role":"assistant","content":"Hello there!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	resp, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "Hello there!" {
		t.Errorf("unexpected content: %+v", resp.Content)
	}
	if resp.StopReason != "stop" {
		t.Errorf("expected stop reason 'stop', got '%s'", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("unexpected usage: %+v", resp.Usage)
	}
}

func TestOllamaComplete_WithSystemPrompt(t *testing.T) {
	var capturedBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"r1","model":"llama3","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		System:   "You are a helpful assistant.",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedBody.Messages) < 2 {
		t.Fatalf("expected system + user messages, got %d", len(capturedBody.Messages))
	}
	if capturedBody.Messages[0].Role != "system" {
		t.Errorf("expected first message role 'system', got '%s'", capturedBody.Messages[0].Role)
	}
}

func TestOllamaComplete_RequestBodyFields(t *testing.T) {
	var capturedBody chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"r1","model":"llama3","choices":[]}`))
	}))
	defer server.Close()

	temp := 0.7
	p := NewOllama(server.URL)
	_, _ = p.Complete(context.Background(), &types.CompletionRequest{
		Model:       "llama3",
		MaxTokens:   512,
		Temperature: temp,
		Messages:    []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})

	if capturedBody.Model != "llama3" {
		t.Errorf("expected model 'llama3', got '%s'", capturedBody.Model)
	}
	if capturedBody.MaxTokens != 512 {
		t.Errorf("expected max_tokens 512, got %d", capturedBody.MaxTokens)
	}
	if capturedBody.Temperature == nil || *capturedBody.Temperature != temp {
		t.Errorf("expected temperature %v, got %v", temp, capturedBody.Temperature)
	}
}

func TestOllamaComplete_ToolRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"r1","model":"llama3","choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"search","arguments":"{\"q\":\"Go concurrency\"}"}}]},"finish_reason":"tool_calls"}]}`))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	resp, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"search for Go concurrency"`)}},
		Tools:    []types.ToolDefinition{{Name: "search", Description: "search the web", InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "tool_use" {
		t.Fatalf("expected 1 tool_use block, got %+v", resp.Content)
	}
	if resp.Content[0].ID != "call-1" || resp.Content[0].Name != "search" {
		t.Errorf("unexpected tool call: %+v", resp.Content[0])
	}
	if string(resp.Content[0].Input) != `{"q":"Go concurrency"}` {
		t.Errorf("unexpected tool input: %s", resp.Content[0].Input)
	}
}

func TestOllamaComplete_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusInternalServerError)
w.Write([]byte(`{"error": "something went wrong"}`))
}))
	defer server.Close()

	p := NewOllama(server.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestOllamaComplete_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewOllama(server.URL)
	_, err := p.Complete(ctx, &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

// --- Stream() ---

func TestOllamaStream_TextDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"id\":\"s1\",\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n"))
		w.Write([]byte("data: {\"id\":\"s1\",\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n"))
		w.Write([]byte("data: {\"id\":\"s1\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2}}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	events, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var text string
	var gotDone bool
	for e := range events {
		if e.Type == types.StreamEventDelta {
			text += e.Content
		}
		if e.Type == types.StreamEventDone {
			gotDone = true
		}
	}
	if text != "Hello world" {
		t.Errorf("expected 'Hello world', got '%s'", text)
	}
	if !gotDone {
		t.Error("expected StreamEventDone")
	}
}

func TestOllamaStream_ToolCallAccumulation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(`data: {"id":"s1","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}` + "\n\n"))
		w.Write([]byte(`data: {"id":"s1","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]},"finish_reason":null}]}` + "\n\n"))
		w.Write([]byte(`data: {"id":"s1","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	events, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"weather?"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var toolBlock *types.ContentBlock
	for e := range events {
		if e.Type == types.StreamEventContentDone && e.ContentBlock != nil && e.ContentBlock.Type == "tool_use" {
			toolBlock = e.ContentBlock
		}
	}
	if toolBlock == nil {
		t.Fatal("expected a tool_use content block")
	}
	if toolBlock.Name != "get_weather" {
		t.Errorf("expected 'get_weather', got '%s'", toolBlock.Name)
	}
	if string(toolBlock.Input) != `{"city":"Paris"}` {
		t.Errorf("expected '{\"city\":\"Paris\"}', got '%s'", toolBlock.Input)
	}
}

func TestOllamaStream_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusInternalServerError)
}))
	defer server.Close()

	p := NewOllama(server.URL)
	_, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error, got nil")
	}
}
