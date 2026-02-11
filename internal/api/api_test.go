package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/langdag/langdag/internal/conversation"
	mockprovider "github.com/langdag/langdag/internal/provider/mock"
	"github.com/langdag/langdag/internal/storage/sqlite"
	"github.com/langdag/langdag/internal/workflow"
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
	workflowMgr := workflow.NewManager(store)

	s := &Server{
		store:       store,
		convMgr:     convMgr,
		workflowMgr: workflowMgr,
		apiKey:      apiKey,
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
