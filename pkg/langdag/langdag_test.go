package langdag_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/langdag/langdag/internal/provider/mock"
	"github.com/langdag/langdag/internal/storage/sqlite"
	"github.com/langdag/langdag/pkg/langdag"
)

// newTestClient creates a Client backed by a temp SQLite DB and a mock provider.
// The mock is configured in "fixed" mode so responses are deterministic.
func newTestClient(t *testing.T, fixedResponse string) *langdag.Client {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatalf("store.Init: %v", err)
	}

	prov := mock.New(mock.Config{
		Mode:          "fixed",
		FixedResponse: fixedResponse,
	})

	client := langdag.NewWithDeps(store, prov)
	t.Cleanup(func() { client.Close() })
	return client
}

// drainStream drains a PromptResult's Stream channel and returns the final
// node ID and the concatenated content.
func drainStream(t *testing.T, result *langdag.PromptResult) (nodeID string, content string) {
	t.Helper()
	for chunk := range result.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.Done {
			nodeID = chunk.NodeID
		} else {
			content += chunk.Content
		}
	}
	return nodeID, content
}

// ---------------------------------------------------------------------------
// New() constructor
// ---------------------------------------------------------------------------

func TestNew_MissingAPIKey(t *testing.T) {
	// Temporarily unset the env var so the library cannot find any key.
	prev := os.Getenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	if prev != "" {
		t.Cleanup(func() { os.Setenv("ANTHROPIC_API_KEY", prev) })
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	_, err := langdag.New(langdag.Config{
		StoragePath: dbPath,
		Provider:    "anthropic",
		APIKeys:     map[string]string{}, // explicitly empty
	})
	if err == nil {
		t.Fatal("expected error when ANTHROPIC_API_KEY is not set")
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	_, err := langdag.New(langdag.Config{
		StoragePath: dbPath,
		Provider:    "does-not-exist",
	})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestNew_WithTempStoragePath(t *testing.T) {
	// The constructor must create the directory and DB file without error.
	// We give it a non-existent nested path; New() should mkdir it.
	dbPath := filepath.Join(t.TempDir(), "nested", "dir", "langdag.db")

	// We still need a valid provider. Inject fake env key only for this test
	// by checking if the env var trick works, OR use openai with a dummy key
	// that reaches the "unknown provider" branch before the key check — no,
	// that's not how it works. Instead, supply a fake key; the provider will
	// be created but never called. This verifies storage path creation only.
	client, err := langdag.New(langdag.Config{
		StoragePath: dbPath,
		Provider:    "anthropic",
		APIKeys:     map[string]string{"anthropic": "sk-test-fake-key"},
	})
	if err != nil {
		t.Fatalf("New() with temp storage path: %v", err)
	}
	t.Cleanup(func() { client.Close() })
}

// ---------------------------------------------------------------------------
// NewWithDeps — basic construction
// ---------------------------------------------------------------------------

func TestNewWithDeps(t *testing.T) {
	client := newTestClient(t, "hello from mock")
	if client == nil {
		t.Fatal("NewWithDeps returned nil")
	}
}

// ---------------------------------------------------------------------------
// PromptOption helpers
// ---------------------------------------------------------------------------

func TestWithModel(t *testing.T) {
	// WithModel is exercised indirectly: providing a custom model string should
	// not cause any errors when the mock provider ignores the model value.
	client := newTestClient(t, "model test response")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "ping", langdag.WithModel("mock-fast"))
	if err != nil {
		t.Fatalf("Prompt with WithModel: %v", err)
	}
	nodeID, content := drainStream(t, result)
	if nodeID == "" {
		t.Error("expected a non-empty nodeID")
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestWithSystemPrompt(t *testing.T) {
	client := newTestClient(t, "system prompt test")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "hello", langdag.WithSystemPrompt("You are a test bot."))
	if err != nil {
		t.Fatalf("Prompt with WithSystemPrompt: %v", err)
	}
	nodeID, _ := drainStream(t, result)
	if nodeID == "" {
		t.Error("expected a non-empty nodeID")
	}
}

func TestWithMaxTokens(t *testing.T) {
	// WithMaxTokens is a no-op for the mock provider, but the option must not
	// cause any error or panic in the library.
	client := newTestClient(t, "max tokens test")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "hello", langdag.WithMaxTokens(512))
	if err != nil {
		t.Fatalf("Prompt with WithMaxTokens: %v", err)
	}
	nodeID, _ := drainStream(t, result)
	if nodeID == "" {
		t.Error("expected a non-empty nodeID")
	}
}

func TestCombinedOptions(t *testing.T) {
	client := newTestClient(t, "combined options test")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "test",
		langdag.WithModel("mock-slow"),
		langdag.WithSystemPrompt("Be concise."),
		langdag.WithMaxTokens(256),
	)
	if err != nil {
		t.Fatalf("Prompt with combined options: %v", err)
	}
	nodeID, content := drainStream(t, result)
	if nodeID == "" {
		t.Error("expected a non-empty nodeID")
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

// ---------------------------------------------------------------------------
// Prompt — new conversation
// ---------------------------------------------------------------------------

func TestPrompt_NewConversation(t *testing.T) {
	const fixedResp = "The answer is 42."
	client := newTestClient(t, fixedResp)
	ctx := context.Background()

	result, err := client.Prompt(ctx, "What is the answer?")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if result == nil {
		t.Fatal("Prompt returned nil result")
	}
	if result.Stream == nil {
		t.Fatal("PromptResult.Stream must not be nil")
	}

	nodeID, content := drainStream(t, result)

	if nodeID == "" {
		t.Error("expected NodeID to be set after stream completes")
	}
	if content != fixedResp {
		t.Errorf("content = %q, want %q", content, fixedResp)
	}

	// After draining, PromptResult.NodeID and PromptResult.Content should be
	// populated as well (set by the goroutine in buildResult).
	if result.NodeID == "" {
		t.Error("PromptResult.NodeID should be set after stream is consumed")
	}
	if result.Content != fixedResp {
		t.Errorf("PromptResult.Content = %q, want %q", result.Content, fixedResp)
	}
}

func TestPrompt_StreamChunks(t *testing.T) {
	// The mock provider emits one chunk per word, so a multi-word response
	// should produce multiple delta chunks.
	const fixedResp = "one two three four five"
	client := newTestClient(t, fixedResp)
	ctx := context.Background()

	result, err := client.Prompt(ctx, "count")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	var chunks int
	var doneCount int
	for chunk := range result.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.Done {
			doneCount++
		} else {
			chunks++
		}
	}

	if chunks == 0 {
		t.Error("expected at least one content chunk")
	}
	if doneCount != 1 {
		t.Errorf("expected exactly 1 Done chunk, got %d", doneCount)
	}
}

// ---------------------------------------------------------------------------
// PromptFrom — continue conversation
// ---------------------------------------------------------------------------

func TestPromptFrom_ContinuesConversation(t *testing.T) {
	client := newTestClient(t, "Hello there.")
	ctx := context.Background()

	// Start a new conversation.
	result1, err := client.Prompt(ctx, "Hi!")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	firstNodeID, _ := drainStream(t, result1)
	if firstNodeID == "" {
		t.Fatal("first nodeID must not be empty")
	}

	// Continue from the assistant node using the same client.
	result2, err := client.PromptFrom(ctx, firstNodeID, "Nice to meet you!")
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}
	secondNodeID, _ := drainStream(t, result2)
	if secondNodeID == "" {
		t.Error("second nodeID must not be empty")
	}
	if secondNodeID == firstNodeID {
		t.Error("second nodeID should differ from first")
	}
}

func TestPromptFrom_InvalidNodeID(t *testing.T) {
	client := newTestClient(t, "response")
	ctx := context.Background()

	_, err := client.PromptFrom(ctx, "nonexistent-node-id-xyz", "hello")
	if err == nil {
		t.Fatal("expected error when continuing from a nonexistent node")
	}
}

// ---------------------------------------------------------------------------
// ListConversations
// ---------------------------------------------------------------------------

func TestListConversations_Empty(t *testing.T) {
	client := newTestClient(t, "resp")
	ctx := context.Background()

	roots, err := client.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(roots) != 0 {
		t.Errorf("expected 0 conversations, got %d", len(roots))
	}
}

func TestListConversations_AfterPrompt(t *testing.T) {
	client := newTestClient(t, "response text")
	ctx := context.Background()

	// Create two independent conversations.
	for _, msg := range []string{"first question", "second question"} {
		result, err := client.Prompt(ctx, msg)
		if err != nil {
			t.Fatalf("Prompt(%q): %v", msg, err)
		}
		drainStream(t, result)
	}

	roots, err := client.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(roots) != 2 {
		t.Errorf("expected 2 conversations, got %d", len(roots))
	}
}

// ---------------------------------------------------------------------------
// GetNode
// ---------------------------------------------------------------------------

func TestGetNode_NotFound(t *testing.T) {
	client := newTestClient(t, "resp")
	ctx := context.Background()

	node, err := client.GetNode(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node != nil {
		t.Error("expected nil for a nonexistent node ID")
	}
}

func TestGetNode_ByFullID(t *testing.T) {
	client := newTestClient(t, "get node response")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "hello")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	nodeID, _ := drainStream(t, result)

	node, err := client.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node == nil {
		t.Fatal("GetNode returned nil for a known node ID")
	}
	if node.ID != nodeID {
		t.Errorf("node.ID = %q, want %q", node.ID, nodeID)
	}
}

func TestGetNode_ByPrefix(t *testing.T) {
	client := newTestClient(t, "prefix lookup response")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "prefix test")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	nodeID, _ := drainStream(t, result)

	// Use the first 8 characters as a prefix.
	prefix := nodeID[:8]
	node, err := client.GetNode(ctx, prefix)
	if err != nil {
		t.Fatalf("GetNode by prefix: %v", err)
	}
	if node == nil {
		t.Fatalf("GetNode by prefix %q returned nil", prefix)
	}
	if node.ID != nodeID {
		t.Errorf("node.ID = %q, want %q", node.ID, nodeID)
	}
}

// ---------------------------------------------------------------------------
// GetSubtree
// ---------------------------------------------------------------------------

func TestGetSubtree_SingleConversation(t *testing.T) {
	client := newTestClient(t, "subtree response")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "root message")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	_, _ = drainStream(t, result)

	// Retrieve the root node (first ListConversations entry).
	roots, err := client.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("expected at least one root")
	}

	subtree, err := client.GetSubtree(ctx, roots[0].ID)
	if err != nil {
		t.Fatalf("GetSubtree: %v", err)
	}
	// A simple Prompt creates: root user node + assistant response node = 2 nodes.
	if len(subtree) < 2 {
		t.Errorf("expected at least 2 nodes in subtree, got %d", len(subtree))
	}
}

func TestGetSubtree_NotFound(t *testing.T) {
	client := newTestClient(t, "resp")
	ctx := context.Background()

	_, err := client.GetSubtree(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent node in GetSubtree")
	}
}

// ---------------------------------------------------------------------------
// GetAncestors
// ---------------------------------------------------------------------------

func TestGetAncestors_SingleTurn(t *testing.T) {
	client := newTestClient(t, "ancestor response")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "ancestor question")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	assistantNodeID, _ := drainStream(t, result)

	ancestors, err := client.GetAncestors(ctx, assistantNodeID)
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	// At minimum: root user node + assistant node.
	if len(ancestors) < 2 {
		t.Errorf("expected at least 2 ancestors, got %d", len(ancestors))
	}
	// First ancestor should be the root (no parent).
	if ancestors[0].ParentID != "" {
		t.Errorf("first ancestor should be root (ParentID==%q), got %q", "", ancestors[0].ParentID)
	}
	// Last ancestor should be the assistant node itself.
	last := ancestors[len(ancestors)-1]
	if last.ID != assistantNodeID {
		t.Errorf("last ancestor ID = %q, want %q", last.ID, assistantNodeID)
	}
}

func TestGetAncestors_MultiTurn(t *testing.T) {
	client := newTestClient(t, "multi-turn response")
	ctx := context.Background()

	// Turn 1
	r1, err := client.Prompt(ctx, "first turn")
	if err != nil {
		t.Fatalf("Prompt turn 1: %v", err)
	}
	assistantNode1, _ := drainStream(t, r1)

	// Turn 2: continue from first assistant node
	r2, err := client.PromptFrom(ctx, assistantNode1, "second turn")
	if err != nil {
		t.Fatalf("PromptFrom turn 2: %v", err)
	}
	assistantNode2, _ := drainStream(t, r2)

	ancestors, err := client.GetAncestors(ctx, assistantNode2)
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	// root user, assistant1, user2, assistant2 = 4 nodes
	if len(ancestors) < 4 {
		t.Errorf("expected at least 4 ancestors for 2-turn conversation, got %d", len(ancestors))
	}
}

func TestGetAncestors_NotFound(t *testing.T) {
	client := newTestClient(t, "resp")
	ctx := context.Background()

	_, err := client.GetAncestors(ctx, "nonexistent-node-id")
	if err == nil {
		t.Fatal("expected error for nonexistent node in GetAncestors")
	}
}

// ---------------------------------------------------------------------------
// DeleteNode
// ---------------------------------------------------------------------------

func TestDeleteNode_RemovesNodeAndDescendants(t *testing.T) {
	client := newTestClient(t, "to be deleted")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "delete me")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	_, _ = drainStream(t, result)

	roots, err := client.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("expected at least one root to delete")
	}

	rootID := roots[0].ID
	if err := client.DeleteNode(ctx, rootID); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}

	// Node should no longer be found.
	node, err := client.GetNode(ctx, rootID)
	if err != nil {
		t.Fatalf("GetNode after delete: %v", err)
	}
	if node != nil {
		t.Error("node still exists after DeleteNode")
	}

	// Conversation list should be empty.
	roots2, err := client.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations after delete: %v", err)
	}
	if len(roots2) != 0 {
		t.Errorf("expected 0 conversations after delete, got %d", len(roots2))
	}
}

func TestDeleteNode_NotFound(t *testing.T) {
	client := newTestClient(t, "resp")
	ctx := context.Background()

	err := client.DeleteNode(ctx, "nonexistent-node-id-abc")
	if err == nil {
		t.Fatal("expected error when deleting a nonexistent node")
	}
}

func TestDeleteNode_OnlyDeletesSubtree(t *testing.T) {
	client := newTestClient(t, "branch response")
	ctx := context.Background()

	// Create conversation A.
	rA, err := client.Prompt(ctx, "conversation A")
	if err != nil {
		t.Fatalf("Prompt A: %v", err)
	}
	nodeA, _ := drainStream(t, rA)

	// Create conversation B.
	rB, err := client.Prompt(ctx, "conversation B")
	if err != nil {
		t.Fatalf("Prompt B: %v", err)
	}
	_, _ = drainStream(t, rB)

	// Delete just conversation A's root.
	roots, err := client.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}

	// Find root of conversation A (the one that produced nodeA).
	nodeAObj, err := client.GetNode(ctx, nodeA)
	if err != nil || nodeAObj == nil {
		t.Fatalf("GetNode for nodeA: %v", err)
	}
	ancestors, err := client.GetAncestors(ctx, nodeA)
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	rootA := ancestors[0].ID

	if err := client.DeleteNode(ctx, rootA); err != nil {
		t.Fatalf("DeleteNode rootA: %v", err)
	}

	// One conversation should remain.
	remaining, err := client.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations after delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining conversation, got %d", len(remaining))
	}
}

// ---------------------------------------------------------------------------
// Streaming — drain edge cases
// ---------------------------------------------------------------------------

func TestPromptResult_StreamIsNeverNil(t *testing.T) {
	client := newTestClient(t, "never nil")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "test nil stream")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if result.Stream == nil {
		t.Fatal("PromptResult.Stream must never be nil")
	}
	drainStream(t, result)
}

func TestPromptResult_DrainTwice(t *testing.T) {
	// Draining the stream a second time should not block — the channel is
	// closed after the first drain, so the loop exits immediately.
	client := newTestClient(t, "drain twice")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "drain")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	drainStream(t, result)

	// Second drain — should complete instantly (closed channel).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range result.Stream {
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second drain of stream blocked unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_ReleasesResources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "close_test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatalf("store.Init: %v", err)
	}
	prov := mock.New(mock.Config{Mode: "fixed", FixedResponse: "close test"})
	client := langdag.NewWithDeps(store, prov)

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	// Closing twice should return an error on the second call (SQLite behaviour)
	// or at least not panic. Either outcome is acceptable.
	dbPath := filepath.Join(t.TempDir(), "close_idem.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatalf("store.Init: %v", err)
	}
	prov := mock.New(mock.Config{Mode: "fixed", FixedResponse: "resp"})
	client := langdag.NewWithDeps(store, prov)

	_ = client.Close()
	// Second close: we just check it does not panic.
	_ = client.Close()
}

// ---------------------------------------------------------------------------
// Storage() and Provider() accessors
// ---------------------------------------------------------------------------

func TestStorageAccessor(t *testing.T) {
	client := newTestClient(t, "accessor test")
	s := client.Storage()
	if s == nil {
		t.Fatal("Storage() returned nil")
	}
}

func TestProviderAccessor(t *testing.T) {
	client := newTestClient(t, "accessor test")
	p := client.Provider()
	if p == nil {
		t.Fatal("Provider() returned nil")
	}
	if p.Name() != "mock" {
		t.Errorf("Provider().Name() = %q, want %q", p.Name(), "mock")
	}
}
