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
	_, err := c.GetNode(context.Background(), "nonexistent")
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

func TestPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/prompt" {
			t.Errorf("expected POST /prompt, got %s %s", r.Method, r.URL.Path)
		}
		var req promptRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Message != "Hello" {
			t.Errorf("expected message Hello, got %s", req.Message)
		}
		json.NewEncoder(w).Encode(promptResponse{
			NodeID:  "node-456",
			Content: "Hi there!",
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	node, err := c.Prompt(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.ID != "node-456" {
		t.Errorf("expected node-456, got %s", node.ID)
	}
	if node.Content != "Hi there!" {
		t.Errorf("expected content 'Hi there!', got %s", node.Content)
	}
}

func TestPromptWithOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req promptRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "mock-fast" {
			t.Errorf("expected model mock-fast, got %s", req.Model)
		}
		if req.SystemPrompt != "Be helpful" {
			t.Errorf("expected system_prompt 'Be helpful', got %s", req.SystemPrompt)
		}
		json.NewEncoder(w).Encode(promptResponse{NodeID: "n-1", Content: "ok"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	_, err := c.Prompt(context.Background(), "Hi", WithModel("mock-fast"), WithSystem("Be helpful"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodePrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/node-1/prompt" {
			t.Errorf("expected /nodes/node-1/prompt, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(promptResponse{NodeID: "node-2", Content: "continued"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	node := &Node{ID: "node-1", client: c}
	result, err := node.Prompt(context.Background(), "more")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "node-2" {
		t.Errorf("expected node-2, got %s", result.ID)
	}
}

func TestListRoots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/nodes" {
			t.Errorf("expected GET /nodes, got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode([]Node{
			{ID: "root-1", Title: "First"},
			{ID: "root-2", Title: "Second"},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	roots, err := c.ListRoots(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}
	if roots[0].ID != "root-1" {
		t.Errorf("expected root-1, got %s", roots[0].ID)
	}
	if roots[0].client == nil {
		t.Error("expected client to be set on returned nodes")
	}
}

func TestGetNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/abc123" {
			t.Errorf("expected /nodes/abc123, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Node{ID: "abc123", Type: NodeTypeAssistant, Content: "hello"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	node, err := c.GetNode(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.ID != "abc123" {
		t.Errorf("expected abc123, got %s", node.ID)
	}
	if node.client == nil {
		t.Error("expected client to be set")
	}
}

func TestGetTree(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/root-1/tree" {
			t.Errorf("expected /nodes/root-1/tree, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]Node{
			{ID: "root-1", Type: NodeTypeUser, Content: "hi"},
			{ID: "child-1", ParentID: "root-1", Type: NodeTypeAssistant, Content: "hello"},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	tree, err := c.GetTree(context.Background(), "root-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(tree.Nodes))
	}
	if tree.Nodes[0].client == nil {
		t.Error("expected client to be set on tree nodes")
	}
}

func TestDeleteNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(DeleteResponse{Status: "deleted", ID: "node-1"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	err := c.DeleteNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
