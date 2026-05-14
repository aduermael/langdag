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

func TestKimiProviderName(t *testing.T) {
	p := NewKimi("test-key", "")
	if p.Name() != "kimi" {
		t.Errorf("expected name 'kimi', got '%s'", p.Name())
	}
}

func TestKimiDefaultBaseURL(t *testing.T) {
	p := NewKimi("test-key", "")
	if p.baseURL != kimiBaseURL {
		t.Errorf("expected default base URL '%s', got '%s'", kimiBaseURL, p.baseURL)
	}
}

func TestKimiCustomBaseURL(t *testing.T) {
	p := NewKimi("test-key", "https://custom.moonshot.example.com/v1")
	if p.baseURL != "https://custom.moonshot.example.com/v1" {
		t.Errorf("expected custom base URL, got '%s'", p.baseURL)
	}
}

func TestKimiBaseURLTrimming(t *testing.T) {
	p := NewKimi("test-key", "https://api.moonshot.ai/v1/")
	if strings.HasSuffix(p.baseURL, "/") {
		t.Error("expected trailing slash to be trimmed from base URL")
	}
}

func TestKimiModels_FetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}
		json.NewEncoder(w).Encode(kimiModelsResponse{
			Data: []kimiModel{
				{ID: "moonshot-v1-8k", ContextLength: 8192},
				{ID: "moonshot-v1-128k", ContextLength: 131072},
			},
		})
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	models := p.Models()

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "moonshot-v1-8k" {
		t.Errorf("expected model ID 'moonshot-v1-8k', got '%s'", models[0].ID)
	}
	if models[0].ContextWindow != 8192 {
		t.Errorf("expected context window 8192, got %d", models[0].ContextWindow)
	}
	if models[1].ContextWindow != 131072 {
		t.Errorf("expected context window 131072, got %d", models[1].ContextWindow)
	}
}

func TestKimiModels_FetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := NewKimi("bad-key", srv.URL)
	models := p.Models()

	if len(models) != 0 {
		t.Errorf("expected 0 models on fetch failure, got %d", len(models))
	}
}

func TestKimiModels_Cached(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(kimiModelsResponse{
			Data: []kimiModel{
				{ID: "moonshot-v1-8k", ContextLength: 8192},
			},
		})
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	p.Models()
	p.Models()
	p.Models()

	if callCount != 1 {
		t.Errorf("expected 1 HTTP call due to caching, got %d", callCount)
	}
}

func TestKimiModels_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(kimiModelsResponse{Data: []kimiModel{}})
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	models := p.Models()
	if len(models) != 0 {
		t.Errorf("expected 0 models for empty data, got %d", len(models))
	}
}

func TestKimiModels_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	models := p.Models()
	if len(models) != 0 {
		t.Errorf("expected 0 models on invalid JSON, got %d", len(models))
	}
}

func TestKimiModels_ConcurrentAccess(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		json.NewEncoder(w).Encode(kimiModelsResponse{
			Data: []kimiModel{
				{ID: "moonshot-v1-8k", ContextLength: 8192},
			},
		})
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)

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

func TestKimiModels_RetryAfterFailure(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"service temporarily unavailable"}`))
			return
		}
		json.NewEncoder(w).Encode(kimiModelsResponse{
			Data: []kimiModel{
				{ID: "moonshot-v1-8k", ContextLength: 8192},
			},
		})
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)

	models := p.Models()
	if len(models) != 0 {
		t.Errorf("expected 0 models on first failed fetch, got %d", len(models))
	}

	models = p.Models()
	if len(models) != 1 {
		t.Errorf("expected 1 model on retry after failure, got %d", len(models))
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (initial failure + retry), got %d", callCount)
	}
}

func TestKimiDoRequest_UsesCorrectEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		stop := "stop"
		content := "hi"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:      "r1",
			Model:   "moonshot-v1-8k",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if gotPath != "/chat/completions" {
		t.Errorf("expected /chat/completions, got %s", gotPath)
	}
}

func TestKimiDoRequest_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		stop := "stop"
		content := "hi"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:      "r1",
			Model:   "moonshot-v1-8k",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	p := NewKimi("my-api-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if gotAuth != "Bearer my-api-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer my-api-key")
	}
}

func TestKimiDoRequest_ContentTypeHeader(t *testing.T) {
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		stop := "stop"
		content := "hi"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:      "r1",
			Model:   "moonshot-v1-8k",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
}

func TestKimiDoRequest_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := NewKimi("bad-key", srv.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on non-200 response, got nil")
	}
}

func TestKimiDoRequest_ErrorContainsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "bad-model",
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

func TestKimiDoRequest_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewKimi("test-key", srv.URL)
	_, err := p.Complete(ctx, &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestKimiComplete_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stop := "stop"
		content := "Hello from Kimi!"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "chatcmpl-kimi-123",
			Model: "moonshot-v1-8k",
			Choices: []choice{
				{Message: responseMessage{Content: &content}, FinishReason: &stop},
			},
			Usage: &usage{PromptTokens: 10, CompletionTokens: 5},
		})
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	resp, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "chatcmpl-kimi-123" {
		t.Errorf("ID = %q, want chatcmpl-kimi-123", resp.ID)
	}
	if resp.StopReason != "stop" {
		t.Errorf("StopReason = %q, want stop", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "Hello from Kimi!" {
		t.Errorf("unexpected content: %+v", resp.Content)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want InputTokens=10 OutputTokens=5", resp.Usage)
	}
}

func TestKimiComplete_InvalidJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on invalid JSON response, got nil")
	}
}

func TestKimiComplete_ToolCallResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stop := "tool_calls"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:    "r1",
			Model: "moonshot-v1-128k",
			Choices: []choice{
				{
					Message: responseMessage{
						ToolCalls: []responseToolCall{
							{
								ID:   "call_1",
								Type: "function",
								Function: responseFunction{
									Name:      "get_weather",
									Arguments: `{"location":"Beijing"}`,
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

	p := NewKimi("test-key", srv.URL)
	resp, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-128k",
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

func TestKimiComplete_RequestBodyFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		stop := "stop"
		content := "ok"
		json.NewEncoder(w).Encode(chatCompletionResponse{
			ID:      "r1",
			Model:   "moonshot-v1-8k",
			Choices: []choice{{Message: responseMessage{Content: &content}, FinishReason: &stop}},
		})
	}))
	defer srv.Close()

	temp := float64(0.7)
	p := NewKimi("test-key", srv.URL)
	p.Complete(context.Background(), &types.CompletionRequest{
		Model:       "moonshot-v1-8k",
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

func TestKimiStream_ReceivesDeltaAndDone(t *testing.T) {
	sseData := `data: {"id":"1","model":"moonshot-v1-8k","choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"1","model":"moonshot-v1-8k","choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	ch, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
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

func TestKimiStream_DoRequestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	ch, err := p.Stream(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error when doRequest fails, got nil")
	}
	if ch != nil {
		t.Error("expected nil channel on error, got non-nil")
	}
}

func TestKimiStream_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewKimi("test-key", srv.URL)
	_, err := p.Stream(ctx, &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestKimiDoRequest_LargeErrorBodyTruncated(t *testing.T) {
	largeErrorBody := strings.Repeat("x", 10000)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(largeErrorBody))
	}))
	defer srv.Close()

	p := NewKimi("test-key", srv.URL)
	_, err := p.Complete(context.Background(), &types.CompletionRequest{
		Model:    "moonshot-v1-8k",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if len(errMsg) > maxErrorBodySize+100 {
		t.Errorf("error message too long (%d bytes), expected truncation at ~%d bytes",
			len(errMsg), maxErrorBodySize)
	}
}
