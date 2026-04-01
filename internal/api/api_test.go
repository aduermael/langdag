package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"langdag.com/langdag/internal/conversation"
	mockprovider "langdag.com/langdag/internal/provider/mock"
	"langdag.com/langdag/internal/storage/sqlite"
)

// testServer creates a Server with a temp SQLite DB and mock provider for testing.
func testServer(t *testing.T, apiKey string) (*Server, *http.ServeMux) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "langdag-api-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := sqlite.New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	prov := mockprovider.New(mockprovider.Config{
		Mode:          "fixed",
		FixedResponse: "Mock response.",
	})

	convMgr := conversation.NewManager(store, prov)

	s := &Server{
		store:   store,
		convMgr: convMgr,
		apiKey:  apiKey,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /prompt", s.authMiddleware(s.handlePrompt))
	mux.HandleFunc("POST /nodes/{id}/prompt", s.authMiddleware(s.handleNodePrompt))
	mux.HandleFunc("GET /nodes", s.authMiddleware(s.handleListNodes))
	mux.HandleFunc("GET /nodes/{id}", s.authMiddleware(s.handleGetNode))
	mux.HandleFunc("GET /nodes/{id}/tree", s.authMiddleware(s.handleGetTree))
	mux.HandleFunc("DELETE /nodes/{id}", s.authMiddleware(s.handleDeleteNode))

	return s, mux
}

func TestHealthEndpoint(t *testing.T) {
	_, mux := testServer(t, "")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health: status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("health: status = %q, want %q", resp["status"], "ok")
	}
}

func TestPromptNewTree(t *testing.T) {
	_, mux := testServer(t, "")

	body := `{"message":"Hello, world!"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("prompt: status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp PromptResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.NodeID == "" {
		t.Error("prompt: node_id is empty")
	}
	if resp.Content == "" {
		t.Error("prompt: content is empty")
	}
}

func TestPromptEmptyMessage(t *testing.T) {
	_, mux := testServer(t, "")

	body := `{"message":""}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("prompt empty: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestPromptInvalidJSON(t *testing.T) {
	_, mux := testServer(t, "")

	req := httptest.NewRequest("POST", "/prompt", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("prompt invalid json: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestPromptFromNode(t *testing.T) {
	_, mux := testServer(t, "")

	// Create a tree first
	body := `{"message":"First message"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("initial prompt: status = %d; body = %s", w.Code, w.Body.String())
	}

	var firstResp PromptResponse
	json.NewDecoder(w.Body).Decode(&firstResp)

	// Continue from the assistant node
	body = `{"message":"Follow-up question"}`
	req = httptest.NewRequest("POST", "/nodes/"+firstResp.NodeID+"/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("node prompt: status = %d; body = %s", w.Code, w.Body.String())
	}

	var secondResp PromptResponse
	json.NewDecoder(w.Body).Decode(&secondResp)
	if secondResp.NodeID == "" {
		t.Error("node prompt: node_id is empty")
	}
	if secondResp.NodeID == firstResp.NodeID {
		t.Error("node prompt: returned same node_id as first prompt")
	}
}

func TestPromptFromNodeNotFound(t *testing.T) {
	_, mux := testServer(t, "")

	body := `{"message":"Hello"}`
	req := httptest.NewRequest("POST", "/nodes/nonexistent-id/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("node prompt not found: status = %d, want %d; body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestListNodes(t *testing.T) {
	_, mux := testServer(t, "")

	// Initially empty
	req := httptest.NewRequest("GET", "/nodes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list nodes: status = %d", w.Code)
	}

	var nodes []NodeResponse
	json.NewDecoder(w.Body).Decode(&nodes)
	if len(nodes) != 0 {
		t.Fatalf("list nodes: got %d, want 0", len(nodes))
	}

	// Create two trees
	for _, msg := range []string{"Tree one", "Tree two"} {
		body := `{"message":"` + msg + `"}`
		req = httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("prompt %q: status = %d", msg, w.Code)
		}
	}

	// Should list 2 root nodes
	req = httptest.NewRequest("GET", "/nodes", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&nodes)
	if len(nodes) != 2 {
		t.Fatalf("list nodes: got %d, want 2", len(nodes))
	}
}

func TestGetNode(t *testing.T) {
	_, mux := testServer(t, "")

	// Create a tree
	body := `{"message":"Get me"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var promptResp PromptResponse
	json.NewDecoder(w.Body).Decode(&promptResp)

	// Get the assistant node
	req = httptest.NewRequest("GET", "/nodes/"+promptResp.NodeID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get node: status = %d; body = %s", w.Code, w.Body.String())
	}

	var node NodeResponse
	json.NewDecoder(w.Body).Decode(&node)
	if node.ID != promptResp.NodeID {
		t.Errorf("get node: ID = %q, want %q", node.ID, promptResp.NodeID)
	}
	if node.NodeType != "assistant" {
		t.Errorf("get node: type = %q, want %q", node.NodeType, "assistant")
	}
}

func TestGetNodeNotFound(t *testing.T) {
	_, mux := testServer(t, "")

	req := httptest.NewRequest("GET", "/nodes/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("get node not found: status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetTree(t *testing.T) {
	_, mux := testServer(t, "")

	// Create a tree
	body := `{"message":"Tree root"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var promptResp PromptResponse
	json.NewDecoder(w.Body).Decode(&promptResp)

	// Get the root node ID by listing roots
	req = httptest.NewRequest("GET", "/nodes", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var roots []NodeResponse
	json.NewDecoder(w.Body).Decode(&roots)
	if len(roots) == 0 {
		t.Fatal("no roots found")
	}

	// Get tree from root
	req = httptest.NewRequest("GET", "/nodes/"+roots[0].ID+"/tree", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get tree: status = %d; body = %s", w.Code, w.Body.String())
	}

	var tree []NodeResponse
	json.NewDecoder(w.Body).Decode(&tree)
	// Should have at least root (user) + assistant = 2 nodes
	if len(tree) < 2 {
		t.Fatalf("get tree: got %d nodes, want >= 2", len(tree))
	}

	// First node should be the root
	if tree[0].ID != roots[0].ID {
		t.Errorf("tree root ID = %q, want %q", tree[0].ID, roots[0].ID)
	}
	if tree[0].NodeType != "user" {
		t.Errorf("tree root type = %q, want %q", tree[0].NodeType, "user")
	}
}

func TestGetTreeNotFound(t *testing.T) {
	_, mux := testServer(t, "")

	req := httptest.NewRequest("GET", "/nodes/nonexistent/tree", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("get tree not found: status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDeleteNode(t *testing.T) {
	_, mux := testServer(t, "")

	// Create a tree
	body := `{"message":"Delete me"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Get the root
	req = httptest.NewRequest("GET", "/nodes", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var roots []NodeResponse
	json.NewDecoder(w.Body).Decode(&roots)
	if len(roots) == 0 {
		t.Fatal("no roots found")
	}

	// Delete the root
	req = httptest.NewRequest("DELETE", "/nodes/"+roots[0].ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete node: status = %d; body = %s", w.Code, w.Body.String())
	}

	var deleteResp map[string]string
	json.NewDecoder(w.Body).Decode(&deleteResp)
	if deleteResp["status"] != "deleted" {
		t.Errorf("delete: status = %q, want %q", deleteResp["status"], "deleted")
	}

	// Verify it's gone
	req = httptest.NewRequest("GET", "/nodes/"+roots[0].ID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("after delete: status = %d, want %d", w.Code, http.StatusNotFound)
	}

	// Root list should be empty
	req = httptest.NewRequest("GET", "/nodes", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&roots)
	if len(roots) != 0 {
		t.Errorf("after delete: %d roots remain", len(roots))
	}
}

func TestDeleteNodeNotFound(t *testing.T) {
	_, mux := testServer(t, "")

	req := httptest.NewRequest("DELETE", "/nodes/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("delete not found: status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAuthMiddleware(t *testing.T) {
	_, mux := testServer(t, "test-secret-key")

	// No auth header → 401
	req := httptest.NewRequest("GET", "/nodes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Wrong key → 401
	req = httptest.NewRequest("GET", "/nodes", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Correct X-API-Key → 200
	req = httptest.NewRequest("GET", "/nodes", nil)
	req.Header.Set("X-API-Key", "test-secret-key")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("correct api key: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Correct Bearer token → 200
	req = httptest.NewRequest("GET", "/nodes", nil)
	req.Header.Set("Authorization", "Bearer test-secret-key")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("correct bearer: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMiddlewareHealthNoAuth(t *testing.T) {
	_, mux := testServer(t, "test-secret-key")

	// Health endpoint should not require auth
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health no auth: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestPromptWithSystemPrompt(t *testing.T) {
	_, mux := testServer(t, "")

	body := `{"message":"Hi","system_prompt":"You are a pirate."}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("prompt with system: status = %d; body = %s", w.Code, w.Body.String())
	}

	var resp PromptResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Verify the root node has the system prompt
	req = httptest.NewRequest("GET", "/nodes", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var roots []NodeResponse
	json.NewDecoder(w.Body).Decode(&roots)
	if len(roots) == 0 {
		t.Fatal("no roots")
	}
	if roots[0].SystemPrompt != "You are a pirate." {
		t.Errorf("system_prompt = %q, want %q", roots[0].SystemPrompt, "You are a pirate.")
	}
}

func TestPromptWithModel(t *testing.T) {
	_, mux := testServer(t, "")

	body := `{"message":"Hi","model":"mock-fast"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("prompt with model: status = %d; body = %s", w.Code, w.Body.String())
	}

	var resp PromptResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// The assistant node should reference the mock-fast model
	req = httptest.NewRequest("GET", "/nodes/"+resp.NodeID, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var node NodeResponse
	json.NewDecoder(w.Body).Decode(&node)
	if node.Model != "mock-fast" {
		t.Errorf("model = %q, want %q", node.Model, "mock-fast")
	}
}

func TestPromptWithTools(t *testing.T) {
	_, mux := testServer(t, "")

	body := `{"message":"What's the weather?","tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}}}}]}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("prompt with tools: status = %d; body = %s", w.Code, w.Body.String())
	}

	var resp PromptResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.NodeID == "" {
		t.Error("prompt with tools: node_id is empty")
	}
	if resp.Content == "" {
		t.Error("prompt with tools: content is empty")
	}
}

func TestBranching(t *testing.T) {
	_, mux := testServer(t, "")

	// Create initial tree
	body := `{"message":"Start"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var firstResp PromptResponse
	json.NewDecoder(w.Body).Decode(&firstResp)

	// Branch from the same node twice
	body = `{"message":"Branch A"}`
	req = httptest.NewRequest("POST", "/nodes/"+firstResp.NodeID+"/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("branch A: status = %d", w.Code)
	}

	body = `{"message":"Branch B"}`
	req = httptest.NewRequest("POST", "/nodes/"+firstResp.NodeID+"/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("branch B: status = %d", w.Code)
	}

	// Get tree from root — should have root + assistant + 2 branches (user+assistant each) = 6+ nodes
	req = httptest.NewRequest("GET", "/nodes", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var roots []NodeResponse
	json.NewDecoder(w.Body).Decode(&roots)

	req = httptest.NewRequest("GET", "/nodes/"+roots[0].ID+"/tree", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var tree []NodeResponse
	json.NewDecoder(w.Body).Decode(&tree)

	// root(user) → assistant → branch_A(user) → branch_A_assistant
	//                       → branch_B(user) → branch_B_assistant
	// = 6 nodes total
	if len(tree) < 6 {
		t.Errorf("branching tree: got %d nodes, want >= 6", len(tree))
	}
}

// testServerWithMock creates a Server with a custom mock provider config.
func testServerWithMock(t *testing.T, apiKey string, mockCfg mockprovider.Config) (*Server, *http.ServeMux) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "langdag-api-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := sqlite.New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	prov := mockprovider.New(mockCfg)
	convMgr := conversation.NewManager(store, prov)

	s := &Server{
		store:   store,
		convMgr: convMgr,
		apiKey:  apiKey,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /prompt", s.authMiddleware(s.handlePrompt))
	mux.HandleFunc("POST /nodes/{id}/prompt", s.authMiddleware(s.handleNodePrompt))
	mux.HandleFunc("GET /nodes", s.authMiddleware(s.handleListNodes))
	mux.HandleFunc("GET /nodes/{id}", s.authMiddleware(s.handleGetNode))
	mux.HandleFunc("GET /nodes/{id}/tree", s.authMiddleware(s.handleGetTree))
	mux.HandleFunc("DELETE /nodes/{id}", s.authMiddleware(s.handleDeleteNode))

	return s, mux
}

// sseEvent represents a parsed SSE event.
type sseEvent struct {
	Type string
	Data string
}

// parseSSEEvents parses SSE response body into event type/data pairs.
func parseSSEEvents(body string) []sseEvent {
	var events []sseEvent
	lines := strings.Split(body, "\n")
	var currentType string
	var dataLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			currentType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		} else if line == "" && currentType != "" {
			events = append(events, sseEvent{
				Type: currentType,
				Data: strings.Join(dataLines, "\n"),
			})
			currentType = ""
			dataLines = nil
		}
	}
	return events
}

// --- Phase 8a: Streaming error mid-response ---

func TestStreamingErrorMidResponse(t *testing.T) {
	_, mux := testServerWithMock(t, "", mockprovider.Config{
		Mode:             "stream_error",
		FixedResponse:    "one two three four five",
		ErrorAfterChunks: 3,
		Error:            fmt.Errorf("provider crashed mid-stream"),
	})

	reqBody := `{"message":"Hello","stream":true}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	events := parseSSEEvents(w.Body.String())
	if len(events) == 0 {
		t.Fatal("no SSE events received")
	}

	// First event: start
	if events[0].Type != "start" {
		t.Errorf("first event type = %q, want %q", events[0].Type, "start")
	}

	// Count deltas — should be exactly 3 (ErrorAfterChunks)
	var deltaCount int
	for _, e := range events {
		if e.Type == "delta" {
			deltaCount++
		}
	}
	if deltaCount != 3 {
		t.Errorf("delta count = %d, want 3", deltaCount)
	}

	// Should contain error event with provider error message
	var foundError bool
	for _, e := range events {
		if e.Type == "error" {
			foundError = true
			if !strings.Contains(e.Data, "provider crashed mid-stream") {
				t.Errorf("error data = %q, want to contain %q", e.Data, "provider crashed mid-stream")
			}
		}
	}
	if !foundError {
		t.Error("no error event found in SSE stream")
	}

	// Should also have done event (partial content saved as node)
	var foundDone bool
	for _, e := range events {
		if e.Type == "done" {
			foundDone = true
			if !strings.Contains(e.Data, "node_id") {
				t.Errorf("done event missing node_id: %s", e.Data)
			}
		}
	}
	if !foundDone {
		t.Error("no done event — partial content should be saved")
	}
}

// --- Phase 8b: Provider failure during streaming ---

func TestStreamingProviderFailure(t *testing.T) {
	_, mux := testServerWithMock(t, "", mockprovider.Config{
		Mode:  "error",
		Error: fmt.Errorf("provider unavailable"),
	})

	reqBody := `{"message":"Hello","stream":true}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Content-Type should be text/event-stream (headers set before Prompt call)
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream prefix", ct)
	}

	// Body should contain SSE error event, not JSON error
	respBody := w.Body.String()
	if !strings.Contains(respBody, "event: error") {
		t.Errorf("expected SSE error event, got: %s", respBody)
	}

	events := parseSSEEvents(respBody)
	if len(events) == 0 {
		t.Fatal("no SSE events parsed")
	}

	// Should have exactly one error event with wrapped provider error
	if events[0].Type != "error" {
		t.Errorf("first event type = %q, want %q", events[0].Type, "error")
	}
	if !strings.Contains(events[0].Data, "provider unavailable") {
		t.Errorf("error data = %q, want to contain %q", events[0].Data, "provider unavailable")
	}

	// Should NOT have start or done events (provider failed before stream started)
	for _, e := range events {
		if e.Type == "start" || e.Type == "done" {
			t.Errorf("unexpected %q event when provider.Stream() fails", e.Type)
		}
	}
}

// --- Phase 8c: Non-streaming error responses ---

func TestNonStreamingProviderError(t *testing.T) {
	_, mux := testServerWithMock(t, "", mockprovider.Config{
		Mode:  "error",
		Error: fmt.Errorf("authentication failed"),
	})

	body := `{"message":"Hello"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["error"] == "" {
		t.Error("error field is empty")
	}
	if !strings.Contains(errResp["error"], "authentication failed") {
		t.Errorf("error = %q, want to contain %q", errResp["error"], "authentication failed")
	}
}

func TestNonStreamingMidStreamError(t *testing.T) {
	// stream_error mode used non-streaming: collectEvents encounters StreamEventError
	_, mux := testServerWithMock(t, "", mockprovider.Config{
		Mode:             "stream_error",
		FixedResponse:    "one two three four",
		ErrorAfterChunks: 2,
		Error:            fmt.Errorf("connection reset"),
	})

	body := `{"message":"Hello"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)
	if !strings.Contains(errResp["error"], "connection reset") {
		t.Errorf("error = %q, want to contain %q", errResp["error"], "connection reset")
	}
}

// --- Phase 8d: Invalid request validation ---

func TestPromptEmptyBody(t *testing.T) {
	_, mux := testServer(t, "")

	req := httptest.NewRequest("POST", "/prompt", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty body: status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["error"] == "" {
		t.Error("error field is empty for nil body")
	}
}

func TestPromptMissingMessageField(t *testing.T) {
	_, mux := testServer(t, "")

	// Valid JSON but no "message" field
	body := `{"model":"test"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing message: status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["error"] != "message is required" {
		t.Errorf("error = %q, want %q", errResp["error"], "message is required")
	}
}

func TestPromptLargeMessage(t *testing.T) {
	_, mux := testServer(t, "")

	// 1MB+ message — should succeed (no server-side limit)
	largeMsg := strings.Repeat("x", 1<<20+1)
	body := `{"message":"` + largeMsg + `"}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("large message: status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String()[:200])
	}

	var resp PromptResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.NodeID == "" {
		t.Error("node_id is empty for large message")
	}
}

func TestPromptStreamingEmptyBody(t *testing.T) {
	_, mux := testServer(t, "")

	// Streaming request with no body
	req := httptest.NewRequest("POST", "/prompt", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should get 400 (validation happens before streaming starts)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("streaming empty body: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- Phase 8e: Auth edge cases ---

func TestAuthEmptyAPIKeyHeader(t *testing.T) {
	_, mux := testServer(t, "test-secret")

	req := httptest.NewRequest("GET", "/nodes", nil)
	req.Header.Set("X-API-Key", "")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("empty X-API-Key: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMalformedBearerTokens(t *testing.T) {
	_, mux := testServer(t, "test-secret")

	tests := []struct {
		name string
		auth string
		want int
	}{
		{"Bearer with trailing space only", "Bearer ", http.StatusUnauthorized},
		{"Bearer without space", "Bearer", http.StatusUnauthorized},
		{"Basic auth scheme", "Basic dGVzdC1zZWNyZXQ=", http.StatusUnauthorized},
		{"Wrong key", "Bearer wrong-key", http.StatusUnauthorized},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/nodes", nil)
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}

			var errResp map[string]string
			json.NewDecoder(w.Body).Decode(&errResp)
			if errResp["error"] != "unauthorized" {
				t.Errorf("error = %q, want %q", errResp["error"], "unauthorized")
			}
		})
	}
}

func TestAuthHealthBypassesWithWrongKey(t *testing.T) {
	_, mux := testServer(t, "test-secret")

	// Health endpoint works even with wrong credentials
	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health with wrong key: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthNoConfigAllEndpointsOpen(t *testing.T) {
	_, mux := testServer(t, "") // no API key configured

	// All endpoints should be accessible without auth
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/nodes"},
		{"GET", "/health"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code == http.StatusUnauthorized {
				t.Errorf("%s %s returned 401 with no API key configured", ep.method, ep.path)
			}
		})
	}
}

// --- Phase 8f: Bug fix tests ---

func TestStreamingErrorWithNewlines(t *testing.T) {
	// Error messages with newlines must not break SSE format.
	// Each line needs its own "data:" prefix per the SSE spec.
	_, mux := testServerWithMock(t, "", mockprovider.Config{
		Mode:  "error",
		Error: fmt.Errorf("line one\nline two\nline three"),
	})

	reqBody := `{"message":"Hello","stream":true}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	events := parseSSEEvents(w.Body.String())
	if len(events) == 0 {
		t.Fatal("no SSE events parsed")
	}

	// The full multi-line error should be preserved in the parsed event data
	var found bool
	for _, e := range events {
		if e.Type == "error" {
			found = true
			if !strings.Contains(e.Data, "line one") {
				t.Errorf("error data missing 'line one': %q", e.Data)
			}
			if !strings.Contains(e.Data, "line two") {
				t.Errorf("error data missing 'line two': %q", e.Data)
			}
			if !strings.Contains(e.Data, "line three") {
				t.Errorf("error data missing 'line three': %q", e.Data)
			}
		}
	}
	if !found {
		t.Error("no error event found")
	}
}

func TestStreamingMidStreamErrorWithNewlines(t *testing.T) {
	// Mid-stream error with newlines in error message
	_, mux := testServerWithMock(t, "", mockprovider.Config{
		Mode:             "stream_error",
		FixedResponse:    "hello world",
		ErrorAfterChunks: 1,
		Error:            fmt.Errorf("server error\ndetails: connection reset"),
	})

	reqBody := `{"message":"Hello","stream":true}`
	req := httptest.NewRequest("POST", "/prompt", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	events := parseSSEEvents(w.Body.String())

	var found bool
	for _, e := range events {
		if e.Type == "error" {
			found = true
			if !strings.Contains(e.Data, "server error") {
				t.Errorf("error data missing 'server error': %q", e.Data)
			}
			if !strings.Contains(e.Data, "details: connection reset") {
				t.Errorf("error data missing 'details: connection reset': %q", e.Data)
			}
		}
	}
	if !found {
		t.Error("no error event found")
	}
}
