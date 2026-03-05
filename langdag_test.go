package langdag_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"langdag.com/langdag"
	"langdag.com/langdag/internal/provider/mock"
	"langdag.com/langdag/internal/storage/sqlite"
	"langdag.com/langdag/types"
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

// ---------------------------------------------------------------------------
// newTestClientWithConfig creates a Client with a custom mock provider config.
// ---------------------------------------------------------------------------

func newTestClientWithConfig(t *testing.T, cfg mock.Config) *langdag.Client {
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

	prov := mock.New(cfg)
	client := langdag.NewWithDeps(store, prov)
	t.Cleanup(func() { client.Close() })
	return client
}

// ---------------------------------------------------------------------------
// WithTools — tool definitions
// ---------------------------------------------------------------------------

func TestWithTools_PassedToProvider(t *testing.T) {
	// WithTools should not cause errors even with the default mock provider.
	client := newTestClient(t, "tools test response")
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a location",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
		},
	}

	result, err := client.Prompt(ctx, "What's the weather?", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt with WithTools: %v", err)
	}
	nodeID, content := drainStream(t, result)
	if nodeID == "" {
		t.Error("expected a non-empty nodeID")
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestWithTools_ToolUseResponse(t *testing.T) {
	// Use the tool_use mock mode to simulate an LLM responding with tool calls.
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Let me check the weather.",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "get_weather",
				Input: json.RawMessage(`{"location":"San Francisco"}`),
			},
		},
	})
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
		},
	}

	result, err := client.Prompt(ctx, "Weather in SF?", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt with tool_use mock: %v", err)
	}

	var gotToolBlock bool
	var gotDone bool
	var stopReason string
	for chunk := range result.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.ContentBlock != nil && chunk.ContentBlock.Type == "tool_use" {
			gotToolBlock = true
			if chunk.ContentBlock.Name != "get_weather" {
				t.Errorf("tool name = %q, want %q", chunk.ContentBlock.Name, "get_weather")
			}
		}
		if chunk.Done {
			gotDone = true
			stopReason = chunk.StopReason
		}
	}

	if !gotToolBlock {
		t.Error("expected a tool_use content block in the stream")
	}
	if !gotDone {
		t.Error("expected Done chunk")
	}
	if stopReason != "tool_use" {
		t.Errorf("StopReason = %q, want %q", stopReason, "tool_use")
	}
	if result.NodeID == "" {
		t.Error("expected NodeID to be set")
	}
}

func TestWithTools_NodeContentContainsToolUse(t *testing.T) {
	// Verify that when the LLM responds with tool_use blocks, the saved node
	// content contains the full JSON content blocks (not just text).
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Calling tool.",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "run_command",
				Input: json.RawMessage(`{"cmd":"ls"}`),
			},
		},
	})
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "run_command",
			Description: "Run a shell command",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`),
		},
	}

	result, err := client.Prompt(ctx, "List files", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	nodeID, _ := drainStream(t, result)

	// Retrieve the saved node and verify content contains tool_use blocks.
	node, err := client.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node == nil {
		t.Fatal("GetNode returned nil")
	}

	// The content should be JSON-encoded content blocks since there are tool_use blocks.
	var blocks []types.ContentBlock
	if err := json.Unmarshal([]byte(node.Content), &blocks); err != nil {
		t.Fatalf("failed to parse node content as content blocks: %v\ncontent: %s", err, node.Content)
	}

	var foundToolUse bool
	for _, b := range blocks {
		if b.Type == "tool_use" && b.Name == "run_command" {
			foundToolUse = true
		}
	}
	if !foundToolUse {
		t.Errorf("expected tool_use block in saved node content, got: %s", node.Content)
	}
}

func TestWithTools_NoToolsStillWorks(t *testing.T) {
	// Passing nil/empty tools should work the same as not passing tools at all.
	client := newTestClient(t, "no tools response")
	ctx := context.Background()

	result, err := client.Prompt(ctx, "hello", langdag.WithTools(nil))
	if err != nil {
		t.Fatalf("Prompt with nil tools: %v", err)
	}
	nodeID, content := drainStream(t, result)
	if nodeID == "" {
		t.Error("expected non-empty nodeID")
	}
	if content != "no tools response" {
		t.Errorf("content = %q, want %q", content, "no tools response")
	}
}

func TestWithTools_PromptFrom(t *testing.T) {
	// Tools should work with PromptFrom (continuing a conversation).
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Using tool.",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "search",
				Input: json.RawMessage(`{"query":"test"}`),
			},
		},
	})
	ctx := context.Background()

	// Start a conversation without tools (use fixed mode temporarily).
	// Actually, the mock is already in tool_use mode, so the first prompt
	// will also return tool blocks. Let's just test the full flow.
	tools := []types.ToolDefinition{
		{
			Name:        "search",
			Description: "Search for something",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		},
	}

	result1, err := client.Prompt(ctx, "Start", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	firstNodeID, _ := drainStream(t, result1)

	// Continue with tools
	result2, err := client.PromptFrom(ctx, firstNodeID, "Continue search", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}

	var gotToolBlock bool
	for chunk := range result2.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.ContentBlock != nil && chunk.ContentBlock.Type == "tool_use" {
			gotToolBlock = true
		}
	}
	if !gotToolBlock {
		t.Error("expected tool_use content block in PromptFrom response")
	}
	if result2.NodeID == "" {
		t.Error("expected NodeID from PromptFrom")
	}
}

func TestWithTools_MultipleTools(t *testing.T) {
	// Multiple tool definitions and multiple tool calls.
	client := newTestClientWithConfig(t, mock.Config{
		Mode: "tool_use",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "get_weather",
				Input: json.RawMessage(`{"location":"NYC"}`),
			},
			{
				Name:  "get_time",
				Input: json.RawMessage(`{"timezone":"EST"}`),
			},
		},
	})
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		{
			Name:        "get_time",
			Description: "Get time",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}

	result, err := client.Prompt(ctx, "weather and time?", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	var toolNames []string
	for chunk := range result.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.ContentBlock != nil && chunk.ContentBlock.Type == "tool_use" {
			toolNames = append(toolNames, chunk.ContentBlock.Name)
		}
	}

	if len(toolNames) != 2 {
		t.Fatalf("expected 2 tool_use blocks, got %d", len(toolNames))
	}
	if toolNames[0] != "get_weather" || toolNames[1] != "get_time" {
		t.Errorf("tool names = %v, want [get_weather, get_time]", toolNames)
	}
}

// ---------------------------------------------------------------------------
// Multi-turn tool use — verifies that buildMessages correctly handles
// JSON content blocks (tool_use and tool_result) in conversation history.
// This is a regression test for the bug where buildMessages wrapped ALL
// node content with fmt.Sprintf("%q"), corrupting JSON array content.
// ---------------------------------------------------------------------------

func TestToolUse_MultiTurnConversation(t *testing.T) {
	// Simulate a full multi-turn tool use flow:
	// 1. User asks a question -> LLM responds with tool_use
	// 2. User sends tool_result via PromptFrom -> LLM responds with final answer
	//
	// Step 2 requires that buildMessages correctly reconstructs the history,
	// passing through JSON content blocks for the assistant's tool_use response.

	// Step 1: LLM responds with tool_use blocks
	toolUseCfg := mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Let me look that up.",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "get_weather",
				Input: json.RawMessage(`{"location":"Paris"}`),
			},
		},
	}
	client := newTestClientWithConfig(t, toolUseCfg)
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather for a location",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
		},
	}

	result1, err := client.Prompt(ctx, "What's the weather in Paris?", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt (turn 1): %v", err)
	}

	// Collect the tool_use block ID from the stream
	var toolUseID string
	var firstNodeID string
	for chunk := range result1.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error (turn 1): %v", chunk.Error)
		}
		if chunk.ContentBlock != nil && chunk.ContentBlock.Type == "tool_use" {
			toolUseID = chunk.ContentBlock.ID
		}
		if chunk.Done {
			firstNodeID = chunk.NodeID
		}
	}
	if firstNodeID == "" {
		t.Fatal("expected nodeID from turn 1")
	}
	if toolUseID == "" {
		t.Fatal("expected tool_use block with ID from turn 1")
	}

	// Verify the assistant node has JSON content blocks stored
	assistantNode, err := client.GetNode(ctx, firstNodeID)
	if err != nil || assistantNode == nil {
		t.Fatalf("GetNode for assistant: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(assistantNode.Content), "[") {
		t.Fatalf("expected assistant node content to be JSON array, got: %s", assistantNode.Content)
	}

	// Step 2: Send tool_result and get a final answer.
	// Build the tool_result content as a JSON array of content blocks,
	// matching the Anthropic API format.
	toolResultContent := `[{"type":"tool_result","tool_use_id":"` + toolUseID + `","content":"Sunny, 22°C"}]`

	// PromptFrom with the tool result. This is the critical step: buildMessages
	// must correctly handle the assistant's JSON content blocks AND this
	// tool_result JSON content in the conversation history.
	result2, err := client.PromptFrom(ctx, firstNodeID, toolResultContent, langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom (turn 2 - tool_result): %v", err)
	}

	secondNodeID, content := drainStream(t, result2)
	if secondNodeID == "" {
		t.Error("expected nodeID from turn 2")
	}
	// The mock is still in tool_use mode, so it will respond with tool blocks again,
	// but the key assertion is that PromptFrom did not error out due to malformed
	// message content in the conversation history.
	_ = content
}

func TestToolUse_ToolResultContentPassedThrough(t *testing.T) {
	// Verify that when a user sends tool_result content (JSON array),
	// it is stored correctly and reconstructed properly in buildMessages.
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Processing tool result.",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "calculator",
				Input: json.RawMessage(`{"expression":"2+2"}`),
			},
		},
	})
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "calculator",
			Description: "Calculate an expression",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`),
		},
	}

	// Turn 1: get tool_use response
	r1, err := client.Prompt(ctx, "Calculate 2+2", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	nodeID1, _ := drainStream(t, r1)

	// Turn 2: send tool_result
	toolResultJSON := `[{"type":"tool_result","tool_use_id":"toolu_000000","content":"4"}]`
	r2, err := client.PromptFrom(ctx, nodeID1, toolResultJSON, langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom with tool_result: %v", err)
	}
	nodeID2, _ := drainStream(t, r2)
	if nodeID2 == "" {
		t.Error("expected nodeID from tool_result turn")
	}

	// Verify the user node that was created for the tool_result has JSON content
	// stored (not double-escaped).
	ancestors, err := client.GetAncestors(ctx, nodeID2)
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	// Ancestors: root_user, assistant_tool_use, user_tool_result, assistant_final
	if len(ancestors) < 4 {
		t.Fatalf("expected at least 4 ancestors, got %d", len(ancestors))
	}

	// The tool_result user node (index 2) should have JSON array content
	toolResultNode := ancestors[2]
	if !strings.HasPrefix(strings.TrimSpace(toolResultNode.Content), "[") {
		t.Errorf("tool_result node content should be JSON array, got: %s", toolResultNode.Content)
	}

	// Turn 3: continue again from the last node — this exercises buildMessages
	// with a full history containing both tool_use and tool_result nodes.
	r3, err := client.PromptFrom(ctx, nodeID2, "What was the result?", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom (turn 3): %v", err)
	}
	nodeID3, _ := drainStream(t, r3)
	if nodeID3 == "" {
		t.Error("expected nodeID from turn 3")
	}
}

func TestToolUse_BuildMessagesWithJSONContentBlocks(t *testing.T) {
	// Unit-style test: create a conversation where the assistant node has JSON
	// content blocks stored, then continue from that node with PromptFrom.
	// This directly exercises buildMessages' handling of JSON array content
	// without requiring the mock to be in tool_use mode for the second call.

	// First, create a conversation with tool_use response using tool_use mode.
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "I will use a tool.",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "lookup",
				Input: json.RawMessage(`{"key":"abc"}`),
			},
		},
	})
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "lookup",
			Description: "Look up a key",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}}}`),
		},
	}

	r1, err := client.Prompt(ctx, "Look up abc", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	assistantNodeID, _ := drainStream(t, r1)

	// Verify the stored content is a JSON array
	node, err := client.GetNode(ctx, assistantNodeID)
	if err != nil || node == nil {
		t.Fatalf("GetNode: %v", err)
	}
	var blocks []types.ContentBlock
	if err := json.Unmarshal([]byte(node.Content), &blocks); err != nil {
		t.Fatalf("assistant node content is not valid JSON content blocks: %v\ncontent: %s", err, node.Content)
	}

	// Now continue from this node. buildMessages must reconstruct the
	// assistant's content as a JSON array (not a quoted string).
	// Even a simple text follow-up should work.
	r2, err := client.PromptFrom(ctx, assistantNodeID, `[{"type":"tool_result","tool_use_id":"toolu_000000","content":"found it: xyz"}]`, langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom after tool_use node: %v", err)
	}
	nodeID2, _ := drainStream(t, r2)
	if nodeID2 == "" {
		t.Error("expected nodeID from PromptFrom")
	}
}

func TestToolUse_PlainTextStartingWithBracket(t *testing.T) {
	// Verify that plain text user messages starting with '[' are NOT
	// incorrectly treated as JSON content blocks.
	client := newTestClientWithConfig(t, mock.Config{
		Mode: "echo", // echoes back the last user message
	})
	ctx := context.Background()

	// These messages start with '[' but are not valid JSON arrays.
	bracketMessages := []string{
		"[IMPORTANT] Please help me with this task",
		"[1] First point [2] Second point",
		"[action required] review the code",
	}

	for _, msg := range bracketMessages {
		result, err := client.Prompt(ctx, msg)
		if err != nil {
			t.Fatalf("Prompt(%q): %v", msg, err)
		}
		_, content := drainStream(t, result)
		// Echo mode should return the original message text, proving it was
		// sent as a JSON string (not misinterpreted as a content block array).
		if content != msg {
			t.Errorf("echo of %q returned %q", msg, content)
		}
	}
}

func TestToolUse_OnlyToolCallsNoText(t *testing.T) {
	// Verify that a tool-use response with NO text (only tool_use blocks)
	// is stored and reconstructed correctly in conversation history.
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "", // no text, only tool calls
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "fetch_data",
				Input: json.RawMessage(`{"url":"https://example.com"}`),
			},
		},
	})
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "fetch_data",
			Description: "Fetch data from a URL",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}}}`),
		},
	}

	// Turn 1: LLM responds with only tool_use (no text)
	r1, err := client.Prompt(ctx, "Fetch example.com", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	nodeID1, _ := drainStream(t, r1)

	// Verify the stored node has JSON content blocks
	node, err := client.GetNode(ctx, nodeID1)
	if err != nil || node == nil {
		t.Fatalf("GetNode: %v", err)
	}
	var blocks []types.ContentBlock
	if err := json.Unmarshal([]byte(node.Content), &blocks); err != nil {
		t.Fatalf("node content is not valid JSON content blocks: %v\ncontent: %s", err, node.Content)
	}
	if len(blocks) == 0 {
		t.Fatal("expected at least one content block")
	}

	// Turn 2: send tool_result and continue — this verifies buildMessages
	// handles the tool-use-only assistant node correctly.
	toolResult := `[{"type":"tool_result","tool_use_id":"toolu_000000","content":"<html>data</html>"}]`
	r2, err := client.PromptFrom(ctx, nodeID1, toolResult, langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom with tool_result: %v", err)
	}
	nodeID2, _ := drainStream(t, r2)
	if nodeID2 == "" {
		t.Error("expected nodeID from turn 2")
	}
}

func TestToolUse_NestedJSONInToolResult(t *testing.T) {
	// Tool results often contain complex nested JSON. Verify this doesn't
	// break content detection or message reconstruction.
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Processing results.",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "query_db",
				Input: json.RawMessage(`{"sql":"SELECT * FROM users"}`),
			},
		},
	})
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "query_db",
			Description: "Query the database",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"sql":{"type":"string"}}}`),
		},
	}

	r1, err := client.Prompt(ctx, "Get all users", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	nodeID1, _ := drainStream(t, r1)

	// Tool result with complex nested JSON content including escaped quotes,
	// arrays, and objects — mimics real-world API responses.
	toolResult := `[{"type":"tool_result","tool_use_id":"toolu_000000","content":"{\"users\":[{\"id\":1,\"name\":\"Alice\"},{\"id\":2,\"name\":\"Bob\"}],\"total\":2}"}]`
	r2, err := client.PromptFrom(ctx, nodeID1, toolResult, langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom with nested JSON tool_result: %v", err)
	}
	nodeID2, _ := drainStream(t, r2)

	// Turn 3: continue conversation to verify the full history (including
	// nested JSON tool result) is correctly reconstructed by buildMessages.
	r3, err := client.PromptFrom(ctx, nodeID2, "How many users?", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom (turn 3): %v", err)
	}
	nodeID3, _ := drainStream(t, r3)
	if nodeID3 == "" {
		t.Error("expected nodeID from turn 3")
	}
}

func TestToolUse_MultipleToolResultsInOneMessage(t *testing.T) {
	// When the LLM calls multiple tools, the user must return all tool_results
	// in a single message. Verify this works correctly.
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Let me check both.",
		ToolCalls: []mock.ToolCallConfig{
			{
				Name:  "get_weather",
				Input: json.RawMessage(`{"location":"NYC"}`),
			},
			{
				Name:  "get_time",
				Input: json.RawMessage(`{"timezone":"EST"}`),
			},
		},
	})
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		{
			Name:        "get_time",
			Description: "Get time",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}

	r1, err := client.Prompt(ctx, "Weather and time in NYC?", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	nodeID1, _ := drainStream(t, r1)

	// Send both tool results in one message (as the Anthropic API requires)
	multiToolResult := `[{"type":"tool_result","tool_use_id":"toolu_000000","content":"Sunny, 75°F"},{"type":"tool_result","tool_use_id":"toolu_000001","content":"3:42 PM EST"}]`
	r2, err := client.PromptFrom(ctx, nodeID1, multiToolResult, langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("PromptFrom with multiple tool_results: %v", err)
	}
	nodeID2, _ := drainStream(t, r2)
	if nodeID2 == "" {
		t.Error("expected nodeID from turn 2")
	}

	// Verify all 4+ nodes exist in the ancestor chain
	ancestors, err := client.GetAncestors(ctx, nodeID2)
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	// root_user, assistant_tool_use, user_tool_results, assistant_response
	if len(ancestors) < 4 {
		t.Errorf("expected at least 4 ancestors, got %d", len(ancestors))
	}
}

func TestStreamNeverHangs_EmptyResponse(t *testing.T) {
	// If the mock provider returns an empty response (no text, no tool calls),
	// the stream must still close and send a Done chunk — never hang.
	client := newTestClientWithConfig(t, mock.Config{
		Mode:          "fixed",
		FixedResponse: "", // empty response
	})
	ctx := context.Background()

	result, err := client.Prompt(ctx, "hello")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Must complete within 5 seconds (should be nearly instant).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range result.Stream {
		}
	}()

	select {
	case <-done:
		// OK — stream closed
	case <-time.After(5 * time.Second):
		t.Fatal("stream hung — timed out waiting for stream to close")
	}
}

func TestWithTools_TextOnlyResponsePreservesPlainText(t *testing.T) {
	// When the LLM responds with text only (no tool_use), the node content
	// should remain as plain text, not JSON.
	client := newTestClient(t, "plain text answer")
	ctx := context.Background()

	tools := []types.ToolDefinition{
		{
			Name:        "calculator",
			Description: "Calculate",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}

	result, err := client.Prompt(ctx, "What is 2+2?", langdag.WithTools(tools))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	nodeID, _ := drainStream(t, result)

	node, err := client.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	// Content should be plain text (not JSON-encoded content blocks)
	// since the response only contained text.
	if strings.HasPrefix(node.Content, "[") {
		t.Errorf("expected plain text content, got JSON: %s", node.Content)
	}
	if node.Content != "plain text answer" {
		t.Errorf("content = %q, want %q", node.Content, "plain text answer")
	}
}
