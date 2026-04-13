package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"langdag.com/langdag/types"
)

func TestOpenRouterProviderName(t *testing.T) {
	p := NewOpenRouter("test-key", "")
	if p.Name() != "openrouter" {
		t.Errorf("expected name 'openrouter', got '%s'", p.Name())
	}
}

func TestOpenRouterDefaultBaseURL(t *testing.T) {
	p := NewOpenRouter("test-key", "")
	if p.baseURL != openRouterBaseURL {
		t.Errorf("expected default base URL '%s', got '%s'", openRouterBaseURL, p.baseURL)
	}
}

func TestOpenRouterCustomBaseURL(t *testing.T) {
	p := NewOpenRouter("test-key", "https://custom.proxy.example.com/v1")
	if p.baseURL != "https://custom.proxy.example.com/v1" {
		t.Errorf("expected custom base URL, got '%s'", p.baseURL)
	}
}

func TestOpenRouterBaseURLTrimming(t *testing.T) {
	p := NewOpenRouter("test-key", "https://openrouter.ai/api/v1/")
	if strings.HasSuffix(p.baseURL, "/") {
		t.Error("expected trailing slash to be trimmed from base URL")
	}
}

func TestOpenRouterModels_FetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}
		if r.Header.Get("HTTP-Referer") == "" {
			t.Error("expected HTTP-Referer header on models request")
		}
		if r.Header.Get("X-Title") == "" {
			t.Error("expected X-Title header on models request")
		}
		json.NewEncoder(w).Encode(openRouterModelsResponse{
			Data: []openRouterModel{
				{ID: "anthropic/claude-sonnet-4-5", Name: "Claude Sonnet 4.5", ContextLength: 200000, TopProvider: struct {
					MaxCompletionTokens int `json:"max_completion_tokens"`
				}{MaxCompletionTokens: 16000}},
				{ID: "openai/gpt-4o", Name: "GPT-4o", ContextLength: 128000, TopProvider: struct {
					MaxCompletionTokens int `json:"max_completion_tokens"`
				}{MaxCompletionTokens: 16384}},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	models := p.Models()

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "anthropic/claude-sonnet-4-5" {
		t.Errorf("expected model ID 'anthropic/claude-sonnet-4-5', got '%s'", models[0].ID)
	}
	if models[0].ContextWindow != 200000 {
		t.Errorf("expected context window 200000, got %d", models[0].ContextWindow)
	}
	if models[0].MaxOutput != 16000 {
		t.Errorf("expected max output 16000, got %d", models[0].MaxOutput)
	}
}

func TestOpenRouterModels_FetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("bad-key", srv.URL)
	models := p.Models()

	if len(models) != 0 {
		t.Errorf("expected 0 models on fetch failure, got %d", len(models))
	}
}

func TestOpenRouterModels_Cached(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(openRouterModelsResponse{
			Data: []openRouterModel{
				{ID: "openai/gpt-4o", Name: "GPT-4o", ContextLength: 128000},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	p.Models()
	p.Models()
	p.Models()

	if callCount != 1 {
		t.Errorf("expected 1 HTTP call due to caching, got %d", callCount)
	}
}

func TestOpenRouterRequiredHeaders(t *testing.T) {
	var gotReferer, gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("HTTP-Referer")
		gotTitle = r.Header.Get("X-Title")
		// Return a minimal valid chat completion response
		stop := "stop"
		content := "hello"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "chatcmpl-1",
			Model: "openai/gpt-4o",
			Choices: []choice{
				{
					Message:      responseMessage{Content: &content},
					FinishReason: &stop,
				},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	p.doRequest(context.Background(), []byte(`{}`)) //nolint — testing headers only

	if gotReferer == "" {
		t.Error("expected HTTP-Referer header to be set")
	}
	if gotTitle == "" {
		t.Error("expected X-Title header to be set")
	}
}

func TestOpenRouterModels_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openRouterModelsResponse{Data: []openRouterModel{}})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	models := p.Models()
	if len(models) != 0 {
		t.Errorf("expected 0 models for empty data, got %d", len(models))
	}
}

func TestOpenRouterModels_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	models := p.Models()
	if len(models) != 0 {
		t.Errorf("expected 0 models on invalid JSON, got %d", len(models))
	}
}

func TestOpenRouterDoRequest_UsesCorrectEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		stop := "stop"
		content := "hi"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "r1",
			Model: "openai/gpt-4o",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if gotPath != "/chat/completions" {
		t.Errorf("expected /chat/completions, got %s", gotPath)
	}
}

func TestOpenRouterDoRequest_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		stop := "stop"
		content := "hi"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "r1",
			Model: "openai/gpt-4o",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("my-api-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if gotAuth != "Bearer my-api-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer my-api-key")
	}
}

func TestOpenRouterDoRequest_ContentTypeHeader(t *testing.T) {
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		stop := "stop"
		content := "hi"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "r1",
			Model: "openai/gpt-4o",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
}

func TestOpenRouterDoRequest_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("bad-key", srv.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on non-200 response, got nil")
	}
}

func TestOpenRouterDoRequest_ErrorContainsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "bad/model",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should contain status 400, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Errorf("error should contain response body, got: %s", err.Error())
	}
}

func TestOpenRouterDoRequest_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewOpenRouter("test-key", srv.URL)
	_, err := p.Complete(ctx, &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestOpenRouterComplete_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stop := "stop"
		content := "Hello from OpenRouter!"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "chatcmpl-or-123",
			Model: "anthropic/claude-sonnet-4-5",
			Choices: []choice{
				{Message: responseMessage{Content: &content}, FinishReason: &stop},
			},
			Usage: &usage{PromptTokens: 10, CompletionTokens: 5},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	resp, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "anthropic/claude-sonnet-4-5",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "chatcmpl-or-123" {
		t.Errorf("ID = %q, want chatcmpl-or-123", resp.ID)
	}
	if resp.StopReason != "stop" {
		t.Errorf("StopReason = %q, want stop", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "Hello from OpenRouter!" {
		t.Errorf("unexpected content: %+v", resp.Content)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want InputTokens=10 OutputTokens=5", resp.Usage)
	}
}

func TestOpenRouterComplete_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on invalid JSON response, got nil")
	}
}

func TestOpenRouterStream_ReceivesDeltaAndDone(t *testing.T) {
	sseData := `data: {"id":"1","model":"openai/gpt-4o","choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"1","model":"openai/gpt-4o","choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	ch, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var text string
	for e := range ch {
		if e.Type == types.StreamEventDelta {
			text += e.Content
		}
	}
	if text != "Hello" {
		t.Errorf("expected delta content \"Hello\", got %q", text)
	}
}

func TestOpenRouterStream_DoRequestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	ch, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error when doRequest fails, got nil")
	}
	if ch != nil {
		t.Error("expected nil channel on error, got non-nil")
	}
}

func TestOpenRouterStream_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewOpenRouter("test-key", srv.URL)
	_, err := p.Stream(ctx, &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestOpenRouterModels_FieldMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openRouterModelsResponse{
			Data: []openRouterModel{
				{
					ID:            "anthropic/claude-sonnet-4-5",
					Name:          "Claude Sonnet 4.5",
					ContextLength: 200000,
					TopProvider: struct {
						MaxCompletionTokens int `json:"max_completion_tokens"`
					}{MaxCompletionTokens: 16000},
				},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	models := p.Models()

	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	m := models[0]
	if m.ID != "anthropic/claude-sonnet-4-5" {
		t.Errorf("ID = %q, want anthropic/claude-sonnet-4-5", m.ID)
	}
	if m.Name != "Claude Sonnet 4.5" {
		t.Errorf("Name = %q, want Claude Sonnet 4.5", m.Name)
	}
	if m.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000", m.ContextWindow)
	}
	if m.MaxOutput != 16000 {
		t.Errorf("MaxOutput = %d, want 16000", m.MaxOutput)
	}
}

func TestOpenRouterModels_ConcurrentAccess(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		json.NewEncoder(w).Encode(openRouterModelsResponse{
			Data: []openRouterModel{
				{ID: "openai/gpt-4o", Name: "GPT-4o", ContextLength: 128000},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			models := p.Models()
			if len(models) == 0 {
				t.Error("expected at least one model")
			}
		}()
	}
	wg.Wait()

	if n := callCount.Load(); n != 1 {
		t.Errorf("expected exactly 1 HTTP call due to mutex caching, got %d", n)
	}
}

func TestOpenRouterComplete_RequestBodyFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		stop := "stop"
		content := "ok"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "r1",
			Model: "openai/gpt-4o",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	temp := float64(0.7)
	p := NewOpenRouter("test-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:       "openai/gpt-4o",
		Messages:    []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		MaxTokens:   512,
		Temperature: temp,
		StopSeqs:    []string{"STOP"},
	})

	if v, _ := gotBody["max_tokens"].(float64); int(v) != 512 {
		t.Errorf("max_tokens = %v, want 512", gotBody["max_tokens"])
	}
	if v, _ := gotBody["temperature"].(float64); v != 0.7 {
		t.Errorf("temperature = %v, want 0.7", gotBody["temperature"])
	}
	stops, _ := gotBody["stop"].([]any)
	if len(stops) != 1 || stops[0] != "STOP" {
		t.Errorf("stop = %v, want [STOP]", gotBody["stop"])
	}
}

func TestOpenRouterComplete_SystemPrompt(t *testing.T) {
	var gotMessages []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		msgs, _ := body["messages"].([]any)
		for _, m := range msgs {
			if mm, ok := m.(map[string]any); ok {
				gotMessages = append(gotMessages, mm)
			}
		}
		stop := "stop"
		content := "ok"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "r1",
			Model: "openai/gpt-4o",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		System:   "You are a helpful assistant.",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})

	if len(gotMessages) == 0 {
		t.Fatal("expected messages in request body, got none")
	}
	if role, _ := gotMessages[0]["role"].(string); role != "system" {
		t.Errorf("first message role = %q, want system", role)
	}
	if content, _ := gotMessages[0]["content"].(string); content != "You are a helpful assistant." {
		t.Errorf("system content = %q, want %q", content, "You are a helpful assistant.")
	}
}

func TestOpenRouterComplete_ToolCallResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stop := "tool_calls"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "r1",
			Model: "openai/gpt-4o",
			Choices: []choice{
				{
					Message: responseMessage{
						ToolCalls: []responseToolCall{
							{
								ID:   "call_1",
								Type: "function",
								Function: responseFunction{
									Name:      "get_weather",
									Arguments: `{"location":"NYC"}`,
								},
							},
						},
					},
					FinishReason: &stop,
				},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	resp, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"what's the weather?"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "tool_use" {
		t.Errorf("content type = %q, want tool_use", resp.Content[0].Type)
	}
	if resp.Content[0].Name != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", resp.Content[0].Name)
	}
	if resp.StopReason != "tool_calls" {
		t.Errorf("StopReason = %q, want tool_calls", resp.StopReason)
	}
}

func TestOpenRouterStream_MalformedSSESkipped(t *testing.T) {
	sseData := `data: {"id":"1","model":"openai/gpt-4o","choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}

data: {invalid json line}

data: {"id":"1","model":"openai/gpt-4o","choices":[{"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"1","model":"openai/gpt-4o","choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	ch, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var text string
	for e := range ch {
		if e.Type == types.StreamEventDelta {
			text += e.Content
		}
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestOpenRouterStream_NonDataLinesIgnored(t *testing.T) {
	sseData := `: this is a comment
event: message

data: {"id":"1","model":"openai/gpt-4o","choices":[{"delta":{"content":"Hi"},"finish_reason":null}]}

random line without prefix

data: {"id":"1","model":"openai/gpt-4o","choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	ch, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var text string
	for e := range ch {
		if e.Type == types.StreamEventDelta {
			text += e.Content
		}
	}
	if text != "Hi" {
		t.Errorf("text = %q, want %q", text, "Hi")
	}
}

func TestOpenRouterModels_RetryAfterFailure(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call fails
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"service temporarily unavailable"}`))
			return
		}
		// Second call succeeds
		json.NewEncoder(w).Encode(openRouterModelsResponse{
			Data: []openRouterModel{
				{ID: "openai/gpt-4o", Name: "GPT-4o", ContextLength: 128000},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	
	// First call should fail and return empty
	models := p.Models()
	if len(models) != 0 {
		t.Errorf("expected 0 models on first failed fetch, got %d", len(models))
	}
	
	// Second call should retry and succeed
	models = p.Models()
	if len(models) != 1 {
		t.Errorf("expected 1 model on retry after failure, got %d", len(models))
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (initial failure + retry), got %d", callCount)
	}
}

func TestOpenRouterDoRequest_LargeErrorBodyTruncated(t *testing.T) {
	// Create an error body larger than maxErrorBodySize (4KB)
	largeErrorBody := strings.Repeat("x", 10000) // 10KB of 'x'
	
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(largeErrorBody))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	
	// Error message should be truncated to maxErrorBodySize
	errMsg := err.Error()
	// The error message includes prefix "openrouter: API error (status 400): "
	// so the body part should be at most maxErrorBodySize
	if len(errMsg) > maxErrorBodySize+100 { // +100 for the prefix and formatting
		t.Errorf("error message too long (%d bytes), expected truncation at ~%d bytes", 
			len(errMsg), maxErrorBodySize)
	}
}

func TestOpenRouterModels_LargeErrorBodyTruncated(t *testing.T) {
	// Create an error body larger than maxErrorBodySize (4KB)
	largeErrorBody := strings.Repeat("y", 8000) // 8KB of 'y'
	
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(largeErrorBody))
	}))
	defer srv.Close()

	p := NewOpenRouter("test-key", srv.URL)
	models := p.Models()
	
	// Should return empty models on error
	if len(models) != 0 {
		t.Errorf("expected 0 models on error, got %d", len(models))
	}
	
	// The internal error should have been truncated (we can't directly test this
	// since Models() doesn't return the error, but the truncation prevents memory issues)
}
