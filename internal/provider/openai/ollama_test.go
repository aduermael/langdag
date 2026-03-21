package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"langdag.com/langdag/types"
)

func TestOllamaProviderName(t *testing.T) {
	p := NewOllama("")
	if p.Name() != "ollama" {
		t.Errorf("expected name 'ollama', got '%s'", p.Name())
	}
}

func TestOllamaDefaultBaseURL(t *testing.T) {
	p := NewOllama("")
	if p.baseURL != "http://localhost:11434" {
		t.Errorf("expected default base URL 'http://localhost:11434', got '%s'", p.baseURL)
	}
}

func TestOllamaCustomBaseURL(t *testing.T) {
	p := NewOllama("http://192.168.1.1:11434")
	if p.baseURL != "http://192.168.1.1:11434" {
		t.Errorf("expected custom base URL, got '%s'", p.baseURL)
	}
}

func TestOllamaBaseURLTrimming(t *testing.T) {
	p := NewOllama("http://localhost:11434/")
	if strings.HasSuffix(p.baseURL, "/") {
		t.Error("expected trailing slash to be trimmed from base URL")
	}
}

func TestOllamaModels_EmptyResponse(t *testing.T) {
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
		t.Fatal("expected empty models slice, got nil")
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

func TestOllamaModels_SingleModel(t *testing.T) {
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
	if models[0].ID != "llama3" {
		t.Errorf("expected ID 'llama3', got '%s'", models[0].ID)
	}
	if models[0].ContextWindow != 8192 {
		t.Errorf("expected context window 8192, got %d", models[0].ContextWindow)
	}
}

func TestOllamaModels_MultipleModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [
				{"name": "llama3"},
				{"name": "mistral"},
				{"name": "qwen2.5:7b"}
			]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"model_info": {"llama.context_length": 4096}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
}

func TestOllamaModels_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	models := p.Models()
	if models != nil {
		t.Errorf("expected nil models on error, got %v", models)
	}
}

func TestOllamaDoRequest_NoAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"test","model":"llama3","choices":[]}`))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	_, _ = p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
}

func TestOllamaComplete_RequestFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"test","model":"llama3","choices":[{"message":{"content":"Hi"},"finish_reason":"stop"}]}`))
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
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
}

func TestOllamaStream_RequestFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`data: {"id":"test","choices":[{"delta":{"content":"Hi"},"finish_reason":null}]}

data: [DONE]

`))
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
	count := 0
	for range events {
		count++
	}
	if count == 0 {
		t.Error("expected at least one event")
	}
}

func TestOllamaContextWindowZero(t *testing.T) {
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
		t.Errorf("expected context window 0, got %d", models[0].ContextWindow)
	}
}

func TestOllamaComplete_Error(t *testing.T) {
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

func TestOllamaStream_Error(t *testing.T) {
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

func TestOllamaModels_DifferentContextWindows(t *testing.T) {
	modelCtx := map[string]int{
		"test-model-a": 8192,
		"test-model-b": 32768,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [
				{"name": "test-model-a"},
				{"name": "test-model-b"}
			]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			ctx := modelCtx[req["name"]]
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"model_info": {"llama.context_length": %d}}`, ctx)))
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

	for _, m := range models {
		expected := modelCtx[m.ID]
		if m.ContextWindow != expected {
			t.Errorf("%s expected %d, got %d", m.ID, expected, m.ContextWindow)
		}
	}
}

func TestOllamaComplete_WithContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "resp-1",
			"model": "llama3",
			"choices": [{
				"message": {"role": "assistant", "content": "Hello there!"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5}
		}`))
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
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Text != "Hello there!" {
		t.Errorf("expected 'Hello there!', got '%s'", resp.Content[0].Text)
	}
	if resp.StopReason != "stop" {
		t.Errorf("expected stop reason 'stop', got '%s'", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("unexpected usage: %+v", resp.Usage)
	}
}

func TestOllamaComplete_WithToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "resp-2",
			"model": "llama3",
			"choices": [{
				"message": {
					"role": "assistant",
					"tool_calls": [{
						"id": "call-1",
						"type": "function",
						"function": {"name": "get_weather", "arguments": "{\"city\":\"Paris\"}"}
					}]
				},
				"finish_reason": "tool_calls"
			}]
		}`))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	resp, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"What's the weather?"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "tool_use" {
		t.Errorf("expected type 'tool_use', got '%s'", resp.Content[0].Type)
	}
	if resp.Content[0].Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got '%s'", resp.Content[0].Name)
	}
}

func TestOllamaStream_WithContent(t *testing.T) {
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

func TestOllamaContextWindow_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "llama3"}, {"name": "llama3"}]}`))
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
	p.Models() // first call populates cache
	p.Models() // second call should use cache for already-seen models

	// llama3 appears twice but /api/show should only be called once due to cache
	if callCount != 1 {
		t.Errorf("expected /api/show to be called once (cache hit), got %d calls", callCount)
	}
}

func TestOllamaContextWindow_NonStandardPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "qwen2.5:7b"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			// qwen uses a different architecture prefix
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
		t.Errorf("expected context window 32768, got %d", models[0].ContextWindow)
	}
}

func TestOllamaContextWindow_ShowError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models": [{"name": "llama3"}]}`))
			return
		}
		if r.URL.Path == "/api/show" {
			w.WriteHeader(http.StatusInternalServerError)
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
	// /api/show error should gracefully fall back to 0
	if models[0].ContextWindow != 0 {
		t.Errorf("expected context window 0 on error, got %d", models[0].ContextWindow)
	}
}
