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

func TestOllamaAPIKeyNotSet(t *testing.T) {
	p := NewOllama("http://localhost:11434")
	if p.apiKey != "" {
		t.Errorf("expected empty API key, got '%s'", p.apiKey)
	}
}

func TestOllamaWithAPIKey(t *testing.T) {
	p := NewOllama("http://localhost:11434")
	p.apiKey = "test-key"
	if p.apiKey != "test-key" {
		t.Errorf("expected API key 'test-key', got '%s'", p.apiKey)
	}
}

func TestNewOllamaWithAPIKey(t *testing.T) {
	p := NewOllamaWithAPIKey("http://localhost:11434", "my-proxy-key")
	if p.apiKey != "my-proxy-key" {
		t.Errorf("expected API key 'my-proxy-key', got '%s'", p.apiKey)
	}
	if p.baseURL != "http://localhost:11434" {
		t.Errorf("expected base URL 'http://localhost:11434', got '%s'", p.baseURL)
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

func TestOllamaDoRequest_NoAuthHeaderWhenNoKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"test","model":"llama3","choices":[]}`))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	p.apiKey = ""
	_, _ = p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "llama3",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
}

func TestOllamaDoRequest_WithAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-secret-key" {
			t.Errorf("expected 'Bearer my-secret-key', got %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"test","model":"llama3","choices":[]}`))
	}))
	defer server.Close()

	p := NewOllama(server.URL)
	p.apiKey = "my-secret-key"
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
