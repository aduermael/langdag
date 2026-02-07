package langdag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:8080")
	if c.baseURL != "http://localhost:8080" {
		t.Errorf("expected baseURL http://localhost:8080, got %s", c.baseURL)
	}
	if c.apiKey != "" {
		t.Errorf("expected empty apiKey, got %s", c.apiKey)
	}
}

func TestNewClient_TrailingSlash(t *testing.T) {
	c := NewClient("http://localhost:8080/")
	if c.baseURL != "http://localhost:8080" {
		t.Errorf("expected baseURL without trailing slash, got %s", c.baseURL)
	}
}

func TestWithAPIKey(t *testing.T) {
	c := NewClient("http://localhost:8080", WithAPIKey("test-key"))
	if c.apiKey != "test-key" {
		t.Errorf("expected apiKey test-key, got %s", c.apiKey)
	}
}

func TestWithBearerToken(t *testing.T) {
	c := NewClient("http://localhost:8080", WithBearerToken("tok-123"))
	if c.bearerToken != "tok-123" {
		t.Errorf("expected bearerToken tok-123, got %s", c.bearerToken)
	}
}

func TestWithTimeout(t *testing.T) {
	c := NewClient("http://localhost:8080", WithTimeout(60*time.Second))
	if c.httpClient.Timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %s", c.httpClient.Timeout)
	}
}

func TestHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	resp, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
}

func TestSetHeaders_APIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "my-key" {
			t.Errorf("expected X-API-Key my-key, got %s", got)
		}
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
	}))
	defer server.Close()

	c := NewClient(server.URL, WithAPIKey("my-key"))
	_, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetHeaders_BearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer my-token" {
			t.Errorf("expected Authorization Bearer my-token, got %s", got)
		}
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
	}))
	defer server.Close()

	c := NewClient(server.URL, WithBearerToken("my-token"))
	_, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPIError_401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	_, err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", apiErr.StatusCode)
	}
	if !apiErr.IsUnauthorized() {
		t.Error("expected IsUnauthorized() to be true")
	}
}

func TestAPIError_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	_, err := c.GetDAG(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !apiErr.IsNotFound() {
		t.Error("expected IsNotFound() to be true")
	}
}

func TestConnectionError(t *testing.T) {
	c := NewClient("http://localhost:1") // port 1 should not be listening
	_, err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	connErr, ok := err.(*ConnectionError)
	if !ok {
		t.Fatalf("expected *ConnectionError, got %T: %v", err, err)
	}
	if connErr.Unwrap() == nil {
		t.Error("expected wrapped error")
	}
}

func TestListDAGs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/dags" {
			t.Errorf("expected GET /dags, got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode([]DAG{
			{ID: "dag-1", Status: DAGStatusCompleted},
			{ID: "dag-2", Status: DAGStatusRunning},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	dags, err := c.ListDAGs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dags) != 2 {
		t.Fatalf("expected 2 dags, got %d", len(dags))
	}
	if dags[0].ID != "dag-1" {
		t.Errorf("expected dag-1, got %s", dags[0].ID)
	}
}

func TestChat_NonStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/chat" {
			t.Errorf("expected POST /chat, got %s %s", r.Method, r.URL.Path)
		}
		var req NewChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Message != "Hello" {
			t.Errorf("expected message Hello, got %s", req.Message)
		}
		json.NewEncoder(w).Encode(ChatResponse{
			DAGID:   "dag-123",
			NodeID:  "node-456",
			Content: "Hi there!",
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	resp, err := c.Chat(context.Background(), &NewChatRequest{
		Message: "Hello",
		Model:   "test",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.DAGID != "dag-123" {
		t.Errorf("expected dag-123, got %s", resp.DAGID)
	}
	if resp.Content != "Hi there!" {
		t.Errorf("expected content 'Hi there!', got %s", resp.Content)
	}
}

func TestDeleteDAG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(DeleteResponse{Status: "deleted", ID: "dag-1"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	resp, err := c.DeleteDAG(context.Background(), "dag-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "deleted" {
		t.Errorf("expected status deleted, got %s", resp.Status)
	}
}
