package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"langdag.com/langdag/internal/provider/mock"
	"langdag.com/langdag/internal/storage/sqlite"
	"langdag.com/langdag/types"
)

func newTestManager(t *testing.T, cfg mock.Config) (*Manager, func()) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	prov := mock.New(cfg)
	mgr := NewManager(store, prov)
	return mgr, func() { store.Close() }
}

// newTestManagerWithMock returns the manager AND the mock provider for request inspection.
func newTestManagerWithMock(t *testing.T, cfg mock.Config) (*Manager, *mock.Provider, func()) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	prov := mock.New(cfg)
	mgr := NewManager(store, prov)
	return mgr, prov, func() { store.Close() }
}

// newTestManagerWithStore returns the manager AND the store for direct DB operations.
func newTestManagerWithStore(t *testing.T, cfg mock.Config) (*Manager, *sqlite.SQLiteStorage, func()) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	prov := mock.New(cfg)
	mgr := NewManager(store, prov)
	return mgr, store, func() { store.Close() }
}

// drainEvents reads all events from a channel and returns them.
func drainEvents(t *testing.T, ch <-chan types.StreamEvent, timeout time.Duration) []types.StreamEvent {
	t.Helper()
	var events []types.StreamEvent
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-timer.C:
			t.Fatal("timed out waiting for events — stream is hanging")
			return events
		}
	}
}

func TestStreamResponse_EmptyProviderStream(t *testing.T) {
	// A provider that returns an immediately-closed channel (no events).
	// The stream must NOT hang — it should close cleanly.
	mgr, cleanup := newTestManager(t, mock.Config{Mode: "fixed", FixedResponse: ""})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Must complete within 5 seconds (should be nearly instant).
	_ = drainEvents(t, events, 5*time.Second)
}

// --- buildMessages unit tests (role merging, node skipping) ---

func TestBuildMessages_MergesConsecutiveUserRoles(t *testing.T) {
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "What's the weather?"},
		{NodeType: types.NodeTypeAssistant, Content: `[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"loc":"NYC"}}]`},
		{NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"toolu_1","content":"Sunny"}]`},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	expectedRoles := []string{"user", "assistant", "user"}
	for i, msg := range messages {
		if msg.Role != expectedRoles[i] {
			t.Errorf("message[%d].Role = %q, want %q", i, msg.Role, expectedRoles[i])
		}
	}
	if messages[1].Content[0] != '[' {
		t.Errorf("assistant message should be JSON array, got: %s", messages[1].Content)
	}
}

func TestBuildMessages_MergesConsecutiveUserWithAppend(t *testing.T) {
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "Start"},
		{NodeType: types.NodeTypeAssistant, Content: `[{"type":"text","text":"Using tool"},{"type":"tool_use","id":"t1","name":"search","input":{}}]`},
		{NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"t1","content":"result"}]`},
		{NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"t2","content":"result2"}]`},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages (user, assistant, merged user), got %d", len(messages))
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(messages[2].Content, &blocks); err != nil {
		t.Fatalf("merged content is not a JSON array: %v\ncontent: %s", err, messages[2].Content)
	}
	if len(blocks) != 2 {
		t.Errorf("expected 2 content blocks in merged message, got %d", len(blocks))
	}
}

func TestBuildMessages_SkipsToolCallNodes(t *testing.T) {
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: "I'll search."},
		{NodeType: types.NodeTypeToolCall, Content: `{"name":"search","input":{}}`},
		{NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"t1","content":"found"}]`},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	roles := []string{"user", "assistant", "user"}
	for i, msg := range messages {
		if msg.Role != roles[i] {
			t.Errorf("message[%d].Role = %q, want %q", i, msg.Role, roles[i])
		}
	}
}

func TestBuildMessages_SystemNodesSkipped(t *testing.T) {
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeSystem, Content: "system message"},
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: "hi"},
	}
	messages := buildMessages(ancestors)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages (system skipped), got %d", len(messages))
	}
}

func TestBuildMessages_EmptyAncestors(t *testing.T) {
	messages := buildMessages(nil)
	if len(messages) != 0 {
		t.Errorf("expected 0 messages for nil ancestors, got %d", len(messages))
	}
}

// --- contentToRawMessage unit tests ---

func TestContentToRawMessage_PlainText(t *testing.T) {
	raw := contentToRawMessage("hello world")
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("expected valid JSON string, got: %s", raw)
	}
	if s != "hello world" {
		t.Errorf("got %q, want %q", s, "hello world")
	}
}

func TestContentToRawMessage_JSONArray(t *testing.T) {
	input := `[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]`
	raw := contentToRawMessage(input)
	if string(raw) != input {
		t.Errorf("got %s, want %s", raw, input)
	}
}

func TestContentToRawMessage_BracketNotJSON(t *testing.T) {
	input := "[IMPORTANT] do this"
	raw := contentToRawMessage(input)
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("expected valid JSON string, got: %s", raw)
	}
	if s != input {
		t.Errorf("got %q, want %q", s, input)
	}
}

func TestContentToRawMessage_EmptyString(t *testing.T) {
	raw := contentToRawMessage("")
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("expected valid JSON string, got: %s", raw)
	}
	if s != "" {
		t.Errorf("got %q, want empty string", s)
	}
}

func TestContentToRawMessage_SpecialChars(t *testing.T) {
	input := "line1\nline2\ttab \"quotes\" and \\backslash"
	raw := contentToRawMessage(input)
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("expected valid JSON string for special chars, got: %s", raw)
	}
	if s != input {
		t.Errorf("got %q, want %q", s, input)
	}
}

// --- mergeContent unit tests ---

func TestMergeContent_StringAndString(t *testing.T) {
	a := json.RawMessage(`"hello"`)
	b := json.RawMessage(`"world"`)
	merged := mergeContent(a, b)
	var blocks []json.RawMessage
	if err := json.Unmarshal(merged, &blocks); err != nil {
		t.Fatalf("merged should be JSON array: %s", merged)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
}

func TestMergeContent_ArrayAndString(t *testing.T) {
	a := json.RawMessage(`[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]`)
	b := json.RawMessage(`"follow up"`)
	merged := mergeContent(a, b)
	var blocks []json.RawMessage
	if err := json.Unmarshal(merged, &blocks); err != nil {
		t.Fatalf("merged should be JSON array: %s", merged)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
}

func TestMergeContent_ArrayAndArray(t *testing.T) {
	a := json.RawMessage(`[{"type":"tool_result","tool_use_id":"t1","content":"r1"}]`)
	b := json.RawMessage(`[{"type":"tool_result","tool_use_id":"t2","content":"r2"}]`)
	merged := mergeContent(a, b)
	var blocks []json.RawMessage
	if err := json.Unmarshal(merged, &blocks); err != nil {
		t.Fatalf("merged should be JSON array: %s", merged)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
}

// --- injectSyntheticToolResults unit tests ---

func TestInjectSyntheticToolResults_NoOrphans(t *testing.T) {
	ancestors := []*types.Node{
		{ID: "n1", NodeType: types.NodeTypeUser},
		{ID: "n2", NodeType: types.NodeTypeAssistant},
	}
	result := injectSyntheticToolResults(ancestors, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 nodes unchanged, got %d", len(result))
	}
}

func TestInjectSyntheticToolResults_InjectsAfterOrphanNode(t *testing.T) {
	ancestors := []*types.Node{
		{ID: "n1", NodeType: types.NodeTypeUser, Content: "hi"},
		{ID: "n2", NodeType: types.NodeTypeAssistant, Content: `[{"type":"tool_use","id":"t1"}]`},
	}
	orphans := map[string][]string{"n2": {"t1"}}
	result := injectSyntheticToolResults(ancestors, orphans)

	if len(result) != 3 {
		t.Fatalf("expected 3 nodes (original 2 + 1 synthetic), got %d", len(result))
	}
	if result[2].NodeType != types.NodeTypeToolResult {
		t.Errorf("injected node type = %q, want tool_result", result[2].NodeType)
	}

	// Verify the synthetic content has the right structure.
	var blocks []struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		IsError   bool   `json:"is_error"`
	}
	if err := json.Unmarshal([]byte(result[2].Content), &blocks); err != nil {
		t.Fatalf("synthetic content not valid JSON: %v", err)
	}
	if len(blocks) != 1 || blocks[0].ToolUseID != "t1" || !blocks[0].IsError {
		t.Errorf("unexpected synthetic content: %+v", blocks)
	}
}

func TestInjectSyntheticToolResults_MultipleOrphansOnSameNode(t *testing.T) {
	ancestors := []*types.Node{
		{ID: "n1", NodeType: types.NodeTypeUser},
		{ID: "n2", NodeType: types.NodeTypeAssistant},
	}
	orphans := map[string][]string{"n2": {"t1", "t2"}}
	result := injectSyntheticToolResults(ancestors, orphans)

	if len(result) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(result))
	}
	var blocks []struct {
		ToolUseID string `json:"tool_use_id"`
	}
	if err := json.Unmarshal([]byte(result[2].Content), &blocks); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(blocks) != 2 {
		t.Errorf("expected 2 synthetic blocks, got %d", len(blocks))
	}
}

// --- extractToolResultIDsFromContent unit tests ---

func TestExtractToolResultIDsFromContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"empty", "", nil},
		{"plain text", "hello", nil},
		{"no tool_result", `[{"type":"text","text":"hi"}]`, nil},
		{"single", `[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]`, []string{"t1"}},
		{"multiple", `[{"type":"tool_result","tool_use_id":"t1","content":"a"},{"type":"tool_result","tool_use_id":"t2","content":"b"}]`, []string{"t1", "t2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolResultIDsFromContent(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- DB-indexed orphaned tool_use integration tests ---
// These test the full flow: create nodes → index tool IDs → detect orphans → fix messages.

func TestOrphanedToolUse_DBIndexed_OrphanedAtEnd(t *testing.T) {
	// The reported bug: assistant has tool_use but conversation ends there.
	// When user continues, PromptFrom should inject synthetic tool_result.
	_, store, cleanup := newTestManagerWithStore(t, mock.Config{Mode: "echo"})
	defer cleanup()
	ctx := context.Background()

	// Create nodes manually to simulate an interrupted conversation.
	userNode := &types.Node{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "What's the weather?", CreatedAt: time.Now()}
	assistantNode := &types.Node{
		ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1,
		NodeType: types.NodeTypeAssistant,
		Content:  `[{"type":"text","text":"Checking..."},{"type":"tool_use","id":"toolu_abc","name":"weather","input":{}}]`,
		CreatedAt: time.Now(),
	}
	if err := store.CreateNode(ctx, userNode); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateNode(ctx, assistantNode); err != nil {
		t.Fatal(err)
	}
	// Index the tool_use ID (normally done by streamResponse).
	if err := store.IndexToolIDs(ctx, "a1", []string{"toolu_abc"}, "use"); err != nil {
		t.Fatal(err)
	}

	// Verify DB detects the orphan.
	orphans, err := store.GetOrphanedToolUses(ctx, []string{"u1", "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || len(orphans["a1"]) != 1 || orphans["a1"][0] != "toolu_abc" {
		t.Fatalf("expected orphan toolu_abc on a1, got: %v", orphans)
	}

	// Now call PromptFrom — it should inject synthetic tool_result.
	mgr := NewManager(store, mock.New(mock.Config{Mode: "echo"}))
	events, err := mgr.PromptFrom(ctx, "a1", "Actually, nevermind", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}
	allEvents := drainEvents(t, events, 5*time.Second)
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventError {
			t.Errorf("unexpected error: %v", ev.Error)
		}
	}
}

func TestOrphanedToolUse_DBIndexed_WithToolResult(t *testing.T) {
	// When tool_result is present, no orphans should be detected.
	_, store, cleanup := newTestManagerWithStore(t, mock.Config{Mode: "echo"})
	defer cleanup()
	ctx := context.Background()

	userNode := &types.Node{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "hi", CreatedAt: time.Now()}
	assistantNode := &types.Node{
		ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1,
		NodeType: types.NodeTypeAssistant,
		Content:  `[{"type":"tool_use","id":"t1","name":"search","input":{}}]`,
		CreatedAt: time.Now(),
	}
	toolResultNode := &types.Node{
		ID: "tr1", ParentID: "a1", RootID: "u1", Sequence: 2,
		NodeType: types.NodeTypeToolResult,
		Content:  `[{"type":"tool_result","tool_use_id":"t1","content":"found"}]`,
		CreatedAt: time.Now(),
	}
	for _, n := range []*types.Node{userNode, assistantNode, toolResultNode} {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.IndexToolIDs(ctx, "a1", []string{"t1"}, "use"); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexToolIDs(ctx, "tr1", []string{"t1"}, "result"); err != nil {
		t.Fatal(err)
	}

	orphans, err := store.GetOrphanedToolUses(ctx, []string{"u1", "a1", "tr1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected no orphans when tool_result present, got: %v", orphans)
	}
}

func TestOrphanedToolUse_DBIndexed_PartialResults(t *testing.T) {
	// 2 tool_use, only 1 has a result → 1 orphan.
	_, store, cleanup := newTestManagerWithStore(t, mock.Config{Mode: "echo"})
	defer cleanup()
	ctx := context.Background()

	userNode := &types.Node{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "compare", CreatedAt: time.Now()}
	assistantNode := &types.Node{
		ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1,
		NodeType: types.NodeTypeAssistant,
		Content:  `[{"type":"tool_use","id":"t1","name":"a","input":{}},{"type":"tool_use","id":"t2","name":"b","input":{}}]`,
		CreatedAt: time.Now(),
	}
	toolResultNode := &types.Node{
		ID: "tr1", ParentID: "a1", RootID: "u1", Sequence: 2,
		NodeType: types.NodeTypeToolResult,
		Content:  `[{"type":"tool_result","tool_use_id":"t1","content":"done"}]`,
		CreatedAt: time.Now(),
	}
	for _, n := range []*types.Node{userNode, assistantNode, toolResultNode} {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.IndexToolIDs(ctx, "a1", []string{"t1", "t2"}, "use"); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexToolIDs(ctx, "tr1", []string{"t1"}, "result"); err != nil {
		t.Fatal(err)
	}

	orphans, err := store.GetOrphanedToolUses(ctx, []string{"u1", "a1", "tr1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans["a1"][0] != "t2" {
		t.Errorf("expected orphan t2 on a1, got: %v", orphans)
	}
}

func TestOrphanedToolUse_DBIndexed_MultipleRounds(t *testing.T) {
	// Round 1: complete. Round 2: orphaned.
	_, store, cleanup := newTestManagerWithStore(t, mock.Config{Mode: "echo"})
	defer cleanup()
	ctx := context.Background()

	nodes := []*types.Node{
		{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "hi", CreatedAt: time.Now()},
		{ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1, NodeType: types.NodeTypeAssistant,
			Content: `[{"type":"tool_use","id":"r1","name":"search","input":{}}]`, CreatedAt: time.Now()},
		{ID: "tr1", ParentID: "a1", RootID: "u1", Sequence: 2, NodeType: types.NodeTypeToolResult,
			Content: `[{"type":"tool_result","tool_use_id":"r1","content":"done"}]`, CreatedAt: time.Now()},
		{ID: "a2", ParentID: "tr1", RootID: "u1", Sequence: 3, NodeType: types.NodeTypeAssistant,
			Content: `[{"type":"tool_use","id":"r2","name":"fetch","input":{}}]`, CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.IndexToolIDs(ctx, "a1", []string{"r1"}, "use"); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexToolIDs(ctx, "tr1", []string{"r1"}, "result"); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexToolIDs(ctx, "a2", []string{"r2"}, "use"); err != nil {
		t.Fatal(err)
	}

	orphans, err := store.GetOrphanedToolUses(ctx, []string{"u1", "a1", "tr1", "a2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans["a2"][0] != "r2" {
		t.Errorf("expected orphan r2 on a2, got: %v", orphans)
	}
}

func TestOrphanedToolUse_DBIndexed_NoToolUse(t *testing.T) {
	// Conversations without tool_use should have zero overhead from orphan detection.
	_, store, cleanup := newTestManagerWithStore(t, mock.Config{Mode: "echo"})
	defer cleanup()
	ctx := context.Background()

	nodes := []*types.Node{
		{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "hi", CreatedAt: time.Now()},
		{ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1, NodeType: types.NodeTypeAssistant, Content: "hello back", CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	orphans, err := store.GetOrphanedToolUses(ctx, []string{"u1", "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected no orphans for text-only conversation, got: %v", orphans)
	}
}

func TestOrphanedToolUse_PromptFromIndexesToolResult(t *testing.T) {
	// Verify that PromptFrom indexes tool_result IDs from the user message.
	_, store, cleanup := newTestManagerWithStore(t, mock.Config{Mode: "echo"})
	defer cleanup()
	ctx := context.Background()

	// Create interrupted conversation.
	nodes := []*types.Node{
		{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "hi", CreatedAt: time.Now()},
		{ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1, NodeType: types.NodeTypeAssistant,
			Content: `[{"type":"tool_use","id":"t1","name":"x","input":{}}]`, CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.IndexToolIDs(ctx, "a1", []string{"t1"}, "use"); err != nil {
		t.Fatal(err)
	}

	// PromptFrom with a tool_result message should index the result.
	mgr := NewManager(store, mock.New(mock.Config{Mode: "echo"}))
	toolResult := `[{"type":"tool_result","tool_use_id":"t1","content":"done"}]`
	events, err := mgr.PromptFrom(ctx, "a1", toolResult, "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}
	drainEvents(t, events, 5*time.Second)

	// Now the tool_use should NOT be orphaned because PromptFrom indexed the result.
	// Get all ancestors for the new user node (u1 → a1 → new_user_node).
	// The new user node was created by PromptFrom, find it.
	children, err := store.GetNodeChildren(ctx, "a1")
	if err != nil {
		t.Fatal(err)
	}
	var userNodeID string
	for _, c := range children {
		if c.NodeType == types.NodeTypeUser {
			userNodeID = c.ID
			break
		}
	}
	if userNodeID == "" {
		t.Fatal("PromptFrom did not create a user node")
	}

	orphans, err := store.GetOrphanedToolUses(ctx, []string{"u1", "a1", userNodeID})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected no orphans after sending tool_result, got: %v", orphans)
	}
}

func TestOrphanedToolUse_NoDuplicateWhenResultSent(t *testing.T) {
	// Regression test for v0.5.6 bug: when PromptFrom sends a tool_result,
	// orphan detection must NOT inject a synthetic duplicate.
	// The bug was that ancestorIDs didn't include the new userNode, so the
	// DB query couldn't see its indexed tool_result IDs.
	_, store, cleanup := newTestManagerWithStore(t, mock.Config{Mode: "echo"})
	defer cleanup()
	ctx := context.Background()

	// Create conversation with tool_use (indexed).
	nodes := []*types.Node{
		{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "hi", CreatedAt: time.Now()},
		{ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1, NodeType: types.NodeTypeAssistant,
			Content: `[{"type":"tool_use","id":"t1","name":"search","input":{}}]`, CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.IndexToolIDs(ctx, "a1", []string{"t1"}, "use"); err != nil {
		t.Fatal(err)
	}

	// Send tool_result via PromptFrom. The mock echoes back whatever we send,
	// so we can inspect the echo to verify no duplicate was injected.
	mgr := NewManager(store, mock.New(mock.Config{Mode: "echo"}))
	toolResult := `[{"type":"tool_result","tool_use_id":"t1","content":"done"}]`
	events, err := mgr.PromptFrom(ctx, "a1", toolResult, "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}

	// Collect the echoed content. The echo mock returns the last user message.
	var echoContent string
	for ev := range events {
		if ev.Type == types.StreamEventError {
			t.Fatalf("unexpected error: %v", ev.Error)
		}
		if ev.Type == types.StreamEventDelta {
			echoContent += ev.Content
		}
	}

	// Count how many times tool_use_id "t1" appears as a tool_result.
	// There must be exactly 1 (the real one), not 2 (real + synthetic).
	count := 0
	for i := 0; i+len(`"tool_use_id":"t1"`) <= len(echoContent); i++ {
		if echoContent[i:i+len(`"tool_use_id":"t1"`)] == `"tool_use_id":"t1"` {
			count++
		}
	}
	// The echo mock echoes the last user message text, not the JSON structure,
	// so instead verify no synthetic node was injected by checking the ancestors.
	children, err := store.GetNodeChildren(ctx, "a1")
	if err != nil {
		t.Fatal(err)
	}
	// Should have exactly 1 child (the user node), not 2.
	userChildren := 0
	for _, c := range children {
		if c.NodeType == types.NodeTypeUser {
			userChildren++
		}
	}
	if userChildren != 1 {
		t.Errorf("expected 1 user child of assistant, got %d", userChildren)
	}

	// Verify via the ancestor chain: get ancestors of the assistant node created
	// by PromptFrom (the grandchild). There should be no synthetic tool_result nodes.
	var assistantChildren []*types.Node
	for _, c := range children {
		if c.NodeType == types.NodeTypeUser {
			gc, err := store.GetNodeChildren(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			assistantChildren = append(assistantChildren, gc...)
		}
	}
	if len(assistantChildren) == 0 {
		t.Fatal("expected assistant child node from PromptFrom")
	}

	// Get full ancestor chain for the final assistant node and rebuild messages.
	ancestors, err := store.GetAncestors(ctx, assistantChildren[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	messages := buildMessages(ancestors)

	// Find the user message that follows the assistant with tool_use.
	// It should contain exactly 1 tool_result for t1, not 2.
	for i, msg := range messages {
		if i == 0 || messages[i-1].Role != "assistant" || msg.Role != "user" {
			continue
		}
		var blocks []struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
		}
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}
		resultCount := 0
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID == "t1" {
				resultCount++
			}
		}
		if resultCount > 1 {
			t.Errorf("DUPLICATE tool_result for t1: found %d in user message (expected 1)\ncontent: %s", resultCount, msg.Content)
		}
	}
}

// --- failingStorage for CreateNode failure tests ---

// failingStorage wraps a real storage and fails CreateNode after N successful calls.
type failingStorage struct {
	inner     Storage
	failAfter int
	calls     int
}

// Storage interface re-export for test (avoids circular imports)
type Storage = interface {
	Init(ctx context.Context) error
	Close() error
	CreateNode(ctx context.Context, node *types.Node) error
	GetNode(ctx context.Context, id string) (*types.Node, error)
	GetNodeByPrefix(ctx context.Context, prefix string) (*types.Node, error)
	GetNodeChildren(ctx context.Context, parentID string) ([]*types.Node, error)
	GetSubtree(ctx context.Context, nodeID string) ([]*types.Node, error)
	GetAncestors(ctx context.Context, nodeID string) ([]*types.Node, error)
	ListRootNodes(ctx context.Context) ([]*types.Node, error)
	UpdateNode(ctx context.Context, node *types.Node) error
	DeleteNode(ctx context.Context, id string) error
	CreateAlias(ctx context.Context, nodeID, alias string) error
	DeleteAlias(ctx context.Context, alias string) error
	GetNodeByAlias(ctx context.Context, alias string) (*types.Node, error)
	ListAliases(ctx context.Context, nodeID string) ([]string, error)
	IndexToolIDs(ctx context.Context, nodeID string, toolIDs []string, role string) error
	GetOrphanedToolUses(ctx context.Context, ancestorIDs []string) (map[string][]string, error)
}

func (f *failingStorage) Init(ctx context.Context) error { return f.inner.Init(ctx) }
func (f *failingStorage) Close() error                   { return f.inner.Close() }
func (f *failingStorage) GetNode(ctx context.Context, id string) (*types.Node, error) {
	return f.inner.GetNode(ctx, id)
}
func (f *failingStorage) GetNodeByPrefix(ctx context.Context, p string) (*types.Node, error) {
	return f.inner.GetNodeByPrefix(ctx, p)
}
func (f *failingStorage) GetNodeChildren(ctx context.Context, p string) ([]*types.Node, error) {
	return f.inner.GetNodeChildren(ctx, p)
}
func (f *failingStorage) GetSubtree(ctx context.Context, id string) ([]*types.Node, error) {
	return f.inner.GetSubtree(ctx, id)
}
func (f *failingStorage) GetAncestors(ctx context.Context, id string) ([]*types.Node, error) {
	return f.inner.GetAncestors(ctx, id)
}
func (f *failingStorage) ListRootNodes(ctx context.Context) ([]*types.Node, error) {
	return f.inner.ListRootNodes(ctx)
}
func (f *failingStorage) UpdateNode(ctx context.Context, node *types.Node) error {
	return f.inner.UpdateNode(ctx, node)
}
func (f *failingStorage) DeleteNode(ctx context.Context, id string) error {
	return f.inner.DeleteNode(ctx, id)
}
func (f *failingStorage) CreateAlias(ctx context.Context, n, a string) error {
	return f.inner.CreateAlias(ctx, n, a)
}
func (f *failingStorage) DeleteAlias(ctx context.Context, a string) error {
	return f.inner.DeleteAlias(ctx, a)
}
func (f *failingStorage) GetNodeByAlias(ctx context.Context, a string) (*types.Node, error) {
	return f.inner.GetNodeByAlias(ctx, a)
}
func (f *failingStorage) ListAliases(ctx context.Context, id string) ([]string, error) {
	return f.inner.ListAliases(ctx, id)
}
func (f *failingStorage) IndexToolIDs(ctx context.Context, nodeID string, toolIDs []string, role string) error {
	return f.inner.IndexToolIDs(ctx, nodeID, toolIDs, role)
}
func (f *failingStorage) GetOrphanedToolUses(ctx context.Context, ancestorIDs []string) (map[string][]string, error) {
	return f.inner.GetOrphanedToolUses(ctx, ancestorIDs)
}

func (f *failingStorage) CreateNode(ctx context.Context, node *types.Node) error {
	f.calls++
	if f.calls > f.failAfter {
		return fmt.Errorf("injected storage failure")
	}
	return f.inner.CreateNode(ctx, node)
}

func TestStreamResponse_CreateNodeFailure_DoesNotHang(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}

	fs := &failingStorage{inner: store, failAfter: 1}
	prov := mock.New(mock.Config{Mode: "fixed", FixedResponse: "test response"})
	mgr := NewManager(fs, prov)
	defer store.Close()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	allEvents := drainEvents(t, events, 5*time.Second)

	var gotError bool
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventError {
			gotError = true
			if ev.Error == nil {
				t.Error("error event has nil Error")
			}
		}
	}
	if !gotError {
		t.Error("expected an error event when CreateNode fails, but got none")
	}
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventNodeSaved {
			t.Error("should not have NodeSaved event when CreateNode fails")
		}
	}
}

func TestStreamResponse_IndexesToolUseIDs(t *testing.T) {
	// Verify that streamResponse indexes tool_use IDs in the DB at write time,
	// so orphan detection doesn't need to parse message JSON.
	_, store, cleanup := newTestManagerWithStore(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Checking.",
		ToolCalls: []mock.ToolCallConfig{
			{Name: "search", Input: json.RawMessage(`{"q":"test"}`)},
		},
	})
	defer cleanup()
	ctx := context.Background()
	mgr := NewManager(store, mock.New(mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Checking.",
		ToolCalls: []mock.ToolCallConfig{
			{Name: "search", Input: json.RawMessage(`{"q":"test"}`)},
		},
	}))

	// Prompt creates a user node + assistant node (with tool_use).
	events, err := mgr.Prompt(ctx, "find it", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	var assistantNodeID string
	for ev := range events {
		if ev.Type == types.StreamEventNodeSaved {
			assistantNodeID = ev.NodeID
		}
	}
	if assistantNodeID == "" {
		t.Fatal("no assistant node saved")
	}

	// Verify the tool_use ID was indexed by streamResponse (not manually).
	// The mock provider generates IDs like "toolu_000000".
	ancestors, err := store.GetAncestors(ctx, assistantNodeID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(ancestors))
	for i, a := range ancestors {
		ids[i] = a.ID
	}
	orphans, err := store.GetOrphanedToolUses(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	// tool_use was indexed but no tool_result → should be orphaned.
	if len(orphans) == 0 {
		t.Fatal("expected orphaned tool_use to be detected via DB index (streamResponse should have indexed it)")
	}
	if _, ok := orphans[assistantNodeID]; !ok {
		t.Errorf("orphan should be on assistant node %s, got: %v", assistantNodeID, orphans)
	}
}

func TestOrphanedToolUse_E2E_PromptThenContinueWithoutResult(t *testing.T) {
	// Full end-to-end: Prompt creates assistant with tool_use (indexed at write time),
	// then PromptFrom continues WITHOUT sending tool_result → orphan detected from DB,
	// synthetic tool_result injected, no API error.
	mgr, cleanup := newTestManager(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Let me check.",
		ToolCalls: []mock.ToolCallConfig{
			{Name: "weather", Input: json.RawMessage(`{"loc":"NYC"}`)},
		},
	})
	defer cleanup()
	ctx := context.Background()

	// Turn 1: user → assistant (with tool_use). tool_use ID indexed by streamResponse.
	events1, err := mgr.Prompt(ctx, "What's the weather?", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	var assistantNodeID string
	for ev := range events1 {
		if ev.Type == types.StreamEventNodeSaved {
			assistantNodeID = ev.NodeID
		}
	}
	if assistantNodeID == "" {
		t.Fatal("no assistant node saved")
	}

	// Turn 2: user continues WITHOUT sending tool_result (the bug scenario).
	// PromptFrom should detect the orphan via DB index and inject synthetic result.
	events2, err := mgr.PromptFrom(ctx, assistantNodeID, "Actually, never mind", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}
	allEvents := drainEvents(t, events2, 5*time.Second)
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventError {
			t.Errorf("unexpected error: %v (orphan detection should have prevented this)", ev.Error)
		}
	}
	// Verify we got a successful response.
	var gotNodeSaved bool
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventNodeSaved {
			gotNodeSaved = true
		}
	}
	if !gotNodeSaved {
		t.Error("expected a NodeSaved event — the conversation should have continued successfully")
	}
}

func TestPromptFrom_ToolResultParent_RolesMerged(t *testing.T) {
	mgr, cleanup := newTestManager(t, mock.Config{
		Mode:          "tool_use",
		FixedResponse: "Using tool.",
		ToolCalls: []mock.ToolCallConfig{
			{Name: "search", Input: json.RawMessage(`{"q":"test"}`)},
		},
	})
	defer cleanup()

	ctx := context.Background()

	// Turn 1: user → assistant (with tool_use)
	events1, err := mgr.Prompt(ctx, "Search for test", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	var assistantNodeID string
	for ev := range events1 {
		if ev.Type == types.StreamEventNodeSaved {
			assistantNodeID = ev.NodeID
		}
	}
	if assistantNodeID == "" {
		t.Fatal("no assistant node saved")
	}

	// Turn 2: send tool_result → assistant
	toolResult := `[{"type":"tool_result","tool_use_id":"toolu_000000","content":"found it"}]`
	events2, err := mgr.PromptFrom(ctx, assistantNodeID, toolResult, "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("PromptFrom with tool_result: %v", err)
	}
	var secondNodeID string
	for ev := range events2 {
		if ev.Type == types.StreamEventNodeSaved {
			secondNodeID = ev.NodeID
		}
	}
	if secondNodeID == "" {
		t.Fatal("no second assistant node saved")
	}

	// Turn 3: plain text continuing from assistant.
	events3, err := mgr.PromptFrom(ctx, secondNodeID, "What did you find?", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("PromptFrom (turn 3): %v", err)
	}
	allEvents := drainEvents(t, events3, 5*time.Second)

	for _, ev := range allEvents {
		if ev.Type == types.StreamEventError {
			t.Errorf("unexpected error event: %v", ev.Error)
		}
	}
}

// --- MaxTokens propagation tests ---

func TestPrompt_MaxTokensPropagated(t *testing.T) {
	mgr, prov, cleanup := newTestManagerWithMock(t, mock.Config{Mode: "fixed", FixedResponse: "ok"})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 12345, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	_ = drainEvents(t, events, 5*time.Second)

	if prov.LastRequest == nil {
		t.Fatal("expected LastRequest to be set")
	}
	if prov.LastRequest.MaxTokens != 12345 {
		t.Errorf("MaxTokens = %d, want 12345", prov.LastRequest.MaxTokens)
	}
}

func TestPrompt_MaxTokensDefaultsTo4096(t *testing.T) {
	mgr, prov, cleanup := newTestManagerWithMock(t, mock.Config{Mode: "fixed", FixedResponse: "ok"})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	_ = drainEvents(t, events, 5*time.Second)

	if prov.LastRequest == nil {
		t.Fatal("expected LastRequest to be set")
	}
	if prov.LastRequest.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", prov.LastRequest.MaxTokens)
	}
}

func TestPromptFrom_MaxTokensPropagated(t *testing.T) {
	mgr, prov, cleanup := newTestManagerWithMock(t, mock.Config{Mode: "fixed", FixedResponse: "ok"})
	defer cleanup()

	ctx := context.Background()
	// Create initial conversation
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	var nodeID string
	for ev := range events {
		if ev.Type == types.StreamEventNodeSaved {
			nodeID = ev.NodeID
		}
	}
	if nodeID == "" {
		t.Fatal("no node ID from initial prompt")
	}

	// Continue with custom maxTokens
	events, err = mgr.PromptFrom(ctx, nodeID, "follow up", "", nil, nil, 9999, 0)
	if err != nil {
		t.Fatalf("PromptFrom: %v", err)
	}
	_ = drainEvents(t, events, 5*time.Second)

	if prov.LastRequest == nil {
		t.Fatal("expected LastRequest to be set")
	}
	if prov.LastRequest.MaxTokens != 9999 {
		t.Errorf("MaxTokens = %d, want 9999", prov.LastRequest.MaxTokens)
	}
}

// --- Empty text block filtering tests ---

func TestToContentBlockArray_EmptyStringReturnsEmptySlice(t *testing.T) {
	// An empty JSON string should produce no content blocks, not a
	// {"type":"text","text":""} block that the Anthropic API rejects.
	raw := json.RawMessage(`""`)
	blocks := toContentBlockArray(raw)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks for empty string, got %d: %s", len(blocks), blocks)
	}
}

func TestToContentBlockArray_NonEmptyStringReturnsTextBlock(t *testing.T) {
	raw := json.RawMessage(`"hello"`)
	blocks := toContentBlockArray(raw)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	var block map[string]string
	if err := json.Unmarshal(blocks[0], &block); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}
	if block["type"] != "text" || block["text"] != "hello" {
		t.Errorf("unexpected block: %v", block)
	}
}

func TestToContentBlockArray_ArrayPassesThrough(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"ok"}]`)
	blocks := toContentBlockArray(raw)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
}

func TestBuildMessages_EmptyAssistantContentMergedAway(t *testing.T) {
	// When an empty assistant node is followed by another assistant node (via
	// mergeContent), the empty text is filtered by toContentBlockArray.
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: ""},
		{NodeType: types.NodeTypeAssistant, Content: "actual response"},
	}

	messages := buildMessages(ancestors)
	// The two assistant nodes should be merged. The empty text from the first
	// should be filtered out by toContentBlockArray, leaving only the second.
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages (user + merged assistant), got %d", len(messages))
	}
	if messages[1].Role != "assistant" {
		t.Fatalf("expected assistant role, got %s", messages[1].Role)
	}
	// Verify no empty text blocks in the merged content.
	var blocks []map[string]string
	if err := json.Unmarshal(messages[1].Content, &blocks); err != nil {
		t.Fatalf("merged content should be a JSON array: %v", err)
	}
	for _, block := range blocks {
		if block["type"] == "text" && block["text"] == "" {
			t.Errorf("merged content contains empty text block: %v", block)
		}
	}
	if len(blocks) != 1 {
		t.Errorf("expected 1 content block after filtering, got %d", len(blocks))
	}
}

func TestBuildMessages_StandaloneEmptyAssistantPassesThrough(t *testing.T) {
	// A standalone empty assistant message passes through buildMessages — this
	// is safe because convertMessages (in the Anthropic provider) filters it
	// before it reaches the API. buildMessages is provider-agnostic.
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: ""},
		{NodeType: types.NodeTypeUser, Content: "follow up"},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	// The empty assistant message exists but has an empty JSON string content.
	// convertMessages will filter this out at the provider level.
	var content string
	if err := json.Unmarshal(messages[1].Content, &content); err != nil {
		t.Fatalf("expected JSON string content: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty content, got %q", content)
	}
}

// --- Output group continuation tests ---

// sequenceResponse describes a single response in a scripted sequence.
type sequenceResponse struct {
	text       string
	stopReason string
	outputToks int
}

// sequenceProvider returns scripted responses in order, implementing the
// provider interface. Thread-safe: each Stream call atomically advances the index.
type sequenceProvider struct {
	responses []sequenceResponse
	callIdx   int
}

func (p *sequenceProvider) Name() string { return "sequence-mock" }
func (p *sequenceProvider) Models() []types.ModelInfo {
	return []types.ModelInfo{{ID: "seq-mock", Name: "Sequence Mock", ContextWindow: 200000, MaxOutput: 8192}}
}
func (p *sequenceProvider) Complete(_ context.Context, _ *types.CompletionRequest) (*types.CompletionResponse, error) {
	return nil, fmt.Errorf("Complete not implemented")
}
func (p *sequenceProvider) Stream(_ context.Context, _ *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	idx := p.callIdx
	p.callIdx++
	if idx >= len(p.responses) {
		return nil, fmt.Errorf("no more scripted responses (call %d)", idx)
	}
	r := p.responses[idx]
	ch := make(chan types.StreamEvent, 10)
	go func() {
		defer close(ch)
		ch <- types.StreamEvent{Type: types.StreamEventStart}
		if r.text != "" {
			ch <- types.StreamEvent{Type: types.StreamEventDelta, Content: r.text}
		}
		outToks := r.outputToks
		if outToks == 0 && r.text != "" {
			outToks = len(r.text)
		}
		stopReason := r.stopReason
		if stopReason == "" {
			stopReason = "end_turn"
		}
		var blocks []types.ContentBlock
		if r.text != "" {
			blocks = append(blocks, types.ContentBlock{Type: "text", Text: r.text})
		}
		ch <- types.StreamEvent{
			Type: types.StreamEventDone,
			Response: &types.CompletionResponse{
				ID: fmt.Sprintf("resp-%d", idx), Model: "seq-mock", Provider: "sequence-mock",
				Content:    blocks,
				StopReason: stopReason,
				Usage:      types.Usage{InputTokens: 100, OutputTokens: outToks},
			},
		}
	}()
	return ch, nil
}

func newTestManagerWithSequence(t *testing.T, responses []sequenceResponse) (*Manager, *sqlite.SQLiteStorage, func()) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	prov := &sequenceProvider{responses: responses}
	mgr := NewManager(store, prov)
	return mgr, store, func() { store.Close() }
}

func TestOutputGroupContinuation_ThreeMaxTokensThenEndTurn(t *testing.T) {
	// Mock: 3 max_tokens responses with text, then 1 end_turn.
	// Expect: 4 nodes all sharing the same OutputGroupID,
	// each with accumulated content, final NodeSaved emitted once.
	mgr, store, cleanup := newTestManagerWithSequence(t, []sequenceResponse{
		{text: "part1", stopReason: "max_tokens", outputToks: 100},
		{text: " part2", stopReason: "max_tokens", outputToks: 100},
		{text: " part3", stopReason: "max_tokens", outputToks: 100},
		{text: " end", stopReason: "end_turn", outputToks: 50},
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "generate a lot", "", "", nil, nil, 1000, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	// Collect all text deltas and count NodeSaved events.
	var allText string
	var savedNodeIDs []string
	for _, ev := range evs {
		switch ev.Type {
		case types.StreamEventDelta:
			allText += ev.Content
		case types.StreamEventNodeSaved:
			savedNodeIDs = append(savedNodeIDs, ev.NodeID)
		case types.StreamEventError:
			t.Fatalf("unexpected error event: %v", ev.Error)
		}
	}

	// Caller sees all text concatenated.
	if allText != "part1 part2 part3 end" {
		t.Errorf("accumulated text = %q, want %q", allText, "part1 part2 part3 end")
	}

	// Only one NodeSaved event (the final node).
	if len(savedNodeIDs) != 1 {
		t.Fatalf("expected 1 NodeSaved event, got %d: %v", len(savedNodeIDs), savedNodeIDs)
	}

	finalNodeID := savedNodeIDs[0]

	// Walk ancestors of the final node — should be user + 4 assistant nodes.
	ancestors, err := store.GetAncestors(ctx, finalNodeID)
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	// 1 user (root) + 4 assistant (continuation chain)
	if len(ancestors) != 5 {
		t.Fatalf("expected 5 ancestors (user + 4 assistant), got %d", len(ancestors))
	}

	// All 4 assistant nodes share the same OutputGroupID.
	var groupID string
	for _, node := range ancestors[1:] {
		if node.NodeType != types.NodeTypeAssistant {
			t.Errorf("expected assistant node, got %s", node.NodeType)
			continue
		}
		if groupID == "" {
			groupID = node.OutputGroupID
		}
		if node.OutputGroupID != groupID {
			t.Errorf("node %s has OutputGroupID %q, want %q", node.ID, node.OutputGroupID, groupID)
		}
	}
	if groupID == "" {
		t.Error("expected non-empty OutputGroupID on continuation nodes")
	}

	// Each node has accumulated content (self-contained).
	expectedContents := []string{
		"part1",
		"part1 part2",
		"part1 part2 part3",
		"part1 part2 part3 end",
	}
	for i, node := range ancestors[1:] {
		if node.Content != expectedContents[i] {
			t.Errorf("node %d content = %q, want %q", i, node.Content, expectedContents[i])
		}
	}

	// The final node has stop_reason = "end_turn".
	finalNode := ancestors[len(ancestors)-1]
	if finalNode.StopReason != "end_turn" {
		t.Errorf("final node stop_reason = %q, want end_turn", finalNode.StopReason)
	}

	// Intermediate nodes have stop_reason = "max_tokens".
	for _, node := range ancestors[1 : len(ancestors)-1] {
		if node.StopReason != "max_tokens" {
			t.Errorf("intermediate node stop_reason = %q, want max_tokens", node.StopReason)
		}
	}
}

func TestOutputGroupContinuation_BudgetExceeded(t *testing.T) {
	// Mock: 3 max_tokens responses each using 500 output tokens.
	// Budget: 1000 tokens. After 2 calls (1000 tokens used), should stop.
	mgr, store, cleanup := newTestManagerWithSequence(t, []sequenceResponse{
		{text: "chunk1", stopReason: "max_tokens", outputToks: 500},
		{text: " chunk2", stopReason: "max_tokens", outputToks: 500},
		{text: " chunk3", stopReason: "end_turn", outputToks: 100}, // should NOT be reached
	})
	defer cleanup()

	ctx := context.Background()
	// maxTokens=500, maxOutputGroupTokens=1000 → budget=1000
	events, err := mgr.Prompt(ctx, "write a lot", "", "", nil, nil, 500, 1000)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var allText string
	var savedNodeIDs []string
	for _, ev := range evs {
		switch ev.Type {
		case types.StreamEventDelta:
			allText += ev.Content
		case types.StreamEventNodeSaved:
			savedNodeIDs = append(savedNodeIDs, ev.NodeID)
		}
	}

	// Should see text from 2 calls only (budget stops before the 3rd).
	if allText != "chunk1 chunk2" {
		t.Errorf("text = %q, want %q", allText, "chunk1 chunk2")
	}

	if len(savedNodeIDs) != 1 {
		t.Fatalf("expected 1 NodeSaved, got %d", len(savedNodeIDs))
	}

	// Should have user + 2 assistant nodes.
	ancestors, err := store.GetAncestors(ctx, savedNodeIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(ancestors) != 3 {
		t.Errorf("expected 3 ancestors (user + 2 assistant), got %d", len(ancestors))
	}

	// Last node has the budget-exceeded stop reason (max_tokens).
	lastNode := ancestors[len(ancestors)-1]
	if lastNode.StopReason != "max_tokens" {
		t.Errorf("last node stop_reason = %q, want max_tokens (budget exceeded)", lastNode.StopReason)
	}
}

func TestOutputGroupContinuation_NoContinuationForToolUse(t *testing.T) {
	// When max_tokens response has tool_use blocks, do NOT continue.
	mgr, cleanup := newTestManager(t, mock.Config{
		Mode:       "tool_use",
		StopReason: "max_tokens",
		ToolCalls: []mock.ToolCallConfig{
			{Name: "read_file", Input: json.RawMessage(`{"path":"foo.txt"}`)},
		},
		FixedResponse: "Let me read that file",
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "read the file", "", "", nil, nil, 1000, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var nodeSaved int
	for _, ev := range evs {
		if ev.Type == types.StreamEventNodeSaved {
			nodeSaved++
		}
	}
	// Should save exactly one node (no continuation for tool_use).
	if nodeSaved != 1 {
		t.Errorf("expected 1 NodeSaved, got %d", nodeSaved)
	}
}

func TestBuildMessages_OutputGroupDeduplication(t *testing.T) {
	// Simulate a conversation with output group nodes. Only the last node
	// in the group should appear in the messages.
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "write something long"},
		{NodeType: types.NodeTypeAssistant, Content: "part1", OutputGroupID: "group-1"},
		{NodeType: types.NodeTypeAssistant, Content: "part1 part2", OutputGroupID: "group-1"},
		{NodeType: types.NodeTypeAssistant, Content: "part1 part2 part3", OutputGroupID: "group-1"},
		{NodeType: types.NodeTypeUser, Content: "thanks"},
	}

	messages := buildMessages(ancestors)

	// Expect: user("write something long"), assistant("part1 part2 part3"), user("thanks")
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// First message: user
	if messages[0].Role != "user" {
		t.Errorf("msg[0] role = %q, want user", messages[0].Role)
	}

	// Second message: assistant with ONLY the final accumulated content
	if messages[1].Role != "assistant" {
		t.Errorf("msg[1] role = %q, want assistant", messages[1].Role)
	}
	var assistantText string
	if err := json.Unmarshal(messages[1].Content, &assistantText); err != nil {
		t.Fatalf("expected string content for assistant message: %v", err)
	}
	if assistantText != "part1 part2 part3" {
		t.Errorf("assistant content = %q, want %q", assistantText, "part1 part2 part3")
	}

	// Third message: user
	if messages[2].Role != "user" {
		t.Errorf("msg[2] role = %q, want user", messages[2].Role)
	}
}

func TestBuildMessages_OutputGroupDedup_NoGroupIDUnchanged(t *testing.T) {
	// Nodes without OutputGroupID should behave as before — no deduplication.
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: "response 1"},
		{NodeType: types.NodeTypeUser, Content: "follow up"},
		{NodeType: types.NodeTypeAssistant, Content: "response 2"},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}
}

func TestOutputGroupContinuation_SingleCallNoGroup(t *testing.T) {
	// When a single call completes with end_turn, no OutputGroupID should be set.
	mgr, store, cleanup := newTestManagerWithSequence(t, []sequenceResponse{
		{text: "complete response", stopReason: "end_turn", outputToks: 50},
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "say hello", "", "", nil, nil, 1000, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var nodeID string
	for _, ev := range evs {
		if ev.Type == types.StreamEventNodeSaved {
			nodeID = ev.NodeID
		}
	}
	if nodeID == "" {
		t.Fatal("no NodeSaved event")
	}

	node, err := store.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if node.OutputGroupID != "" {
		t.Errorf("single-call node should have empty OutputGroupID, got %q", node.OutputGroupID)
	}
}

// =============================================================================
// Phase 3: Conversation & Storage Error Branches
// =============================================================================

// --- 3a: Provider.Stream() returns error ---

func TestPrompt_ProviderStreamError_ReturnsError(t *testing.T) {
	// When the provider's Stream() method itself returns an error (not a stream
	// event error), Prompt() should return that error immediately.
	mgr, cleanup := newTestManager(t, mock.Config{
		Mode:  "error",
		Error: fmt.Errorf("provider unavailable: 503 Service Unavailable"),
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 0, 0)
	if err == nil {
		t.Fatal("expected error from Prompt when provider.Stream() fails, got nil")
	}
	if events != nil {
		t.Error("expected nil events channel when Prompt returns error")
	}
	if !strings.Contains(err.Error(), "failed to stream response") {
		t.Errorf("error should wrap with 'failed to stream response', got: %v", err)
	}
	if !strings.Contains(err.Error(), "503 Service Unavailable") {
		t.Errorf("error should contain original provider error, got: %v", err)
	}
}

func TestPromptFrom_ProviderStreamError_ReturnsError(t *testing.T) {
	// Same test but for PromptFrom — provider.Stream() error should propagate.
	mgr, store, cleanup := newTestManagerWithStore(t, mock.Config{Mode: "fixed", FixedResponse: "ok"})
	defer cleanup()
	ctx := context.Background()

	// Create an initial conversation node so we have a parent to continue from.
	root := &types.Node{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "hi", CreatedAt: time.Now()}
	assistant := &types.Node{ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1, NodeType: types.NodeTypeAssistant, Content: "hello", CreatedAt: time.Now()}
	for _, n := range []*types.Node{root, assistant} {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	// Now replace the provider with one that errors.
	errProv := mock.New(mock.Config{
		Mode:  "error",
		Error: fmt.Errorf("auth failed: 401 Unauthorized"),
	})
	mgr = NewManager(store, errProv)

	events, err := mgr.PromptFrom(ctx, "a1", "continue", "", nil, nil, 0, 0)
	if err == nil {
		t.Fatal("expected error from PromptFrom when provider.Stream() fails")
	}
	if events != nil {
		t.Error("expected nil events channel")
	}
	if !strings.Contains(err.Error(), "failed to stream response") {
		t.Errorf("error should wrap with 'failed to stream response', got: %v", err)
	}
	if !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Errorf("error should contain original provider error, got: %v", err)
	}
}

func TestStreamResponse_StreamEventError_EmittedAndChannelClosed(t *testing.T) {
	// When the provider's stream emits a StreamEventError (mid-stream failure),
	// verify the error is forwarded on the events channel and the channel closes.
	mgr, cleanup := newTestManager(t, mock.Config{
		Mode:             "stream_error",
		FixedResponse:    "word1 word2 word3 word4 word5",
		ErrorAfterChunks: 2,
		Error:            fmt.Errorf("connection reset by peer"),
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt should not return error for stream_error mode: %v", err)
	}

	evs := drainEvents(t, events, 5*time.Second)

	// Should have received some delta events before the error.
	var deltaCount int
	var gotError bool
	var errMsg string
	for _, ev := range evs {
		switch ev.Type {
		case types.StreamEventDelta:
			deltaCount++
		case types.StreamEventError:
			gotError = true
			if ev.Error != nil {
				errMsg = ev.Error.Error()
			}
		}
	}

	if deltaCount == 0 {
		t.Error("expected at least 1 delta event before the error")
	}
	if !gotError {
		t.Fatal("expected a StreamEventError from the stream")
	}
	if !strings.Contains(errMsg, "connection reset by peer") {
		t.Errorf("error message should contain original error, got: %q", errMsg)
	}
	// Channel should be closed (drainEvents completed without timeout).
}

// --- 3b: Database failure mid-stream ---

func TestStreamResponse_CreateNodeFailure_DuringContinuation(t *testing.T) {
	// First continuation saves successfully, second CreateNode call fails.
	// failAfter=2: user node (1) + first assistant (2) succeed, second assistant (3) fails.
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	defer store.Close()

	fs := &failingStorage{inner: store, failAfter: 2}
	prov := &sequenceProvider{responses: []sequenceResponse{
		{text: "part1", stopReason: "max_tokens", outputToks: 100},
		{text: " part2", stopReason: "end_turn", outputToks: 100},
	}}
	mgr := NewManager(fs, prov)

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "generate", "", "", nil, nil, 1000, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	evs := drainEvents(t, events, 5*time.Second)

	// Should get delta events from both calls plus an error event.
	var allText string
	var gotError bool
	var gotNodeSaved bool
	for _, ev := range evs {
		switch ev.Type {
		case types.StreamEventDelta:
			allText += ev.Content
		case types.StreamEventError:
			gotError = true
			if ev.Error == nil {
				t.Error("error event has nil Error")
			} else if !strings.Contains(ev.Error.Error(), "failed to save assistant node") {
				t.Errorf("expected 'failed to save assistant node' error, got: %v", ev.Error)
			}
		case types.StreamEventNodeSaved:
			gotNodeSaved = true
		}
	}

	// We should have received deltas from the first call at least.
	if allText == "" {
		t.Error("expected some delta text before the DB failure")
	}
	if !gotError {
		t.Error("expected an error event when CreateNode fails during continuation")
	}
	// No NodeSaved for the second call (it failed to save).
	// The first node was saved successfully, but since the continuation failed,
	// no NodeSaved event is emitted for it either (the error terminates the goroutine).
	if gotNodeSaved {
		t.Error("should not have NodeSaved when the continuation CreateNode fails")
	}
}

func TestStreamResponse_CreateNodeFailure_FirstCall_NoHang(t *testing.T) {
	// Verify that when CreateNode fails on the very first assistant node,
	// the stream emits an error and closes without hanging — even when the
	// provider sends a valid response with content.
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	defer store.Close()

	// failAfter=1: user node succeeds, assistant node fails.
	fs := &failingStorage{inner: store, failAfter: 1}
	prov := mock.New(mock.Config{Mode: "fixed", FixedResponse: "This should not be saved"})
	mgr := NewManager(fs, prov)

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	evs := drainEvents(t, events, 5*time.Second)

	var gotError bool
	for _, ev := range evs {
		if ev.Type == types.StreamEventError {
			gotError = true
		}
		if ev.Type == types.StreamEventNodeSaved {
			t.Error("should not have NodeSaved when CreateNode fails")
		}
	}
	if !gotError {
		t.Error("expected error event when assistant CreateNode fails")
	}
}

// --- 3c: Malformed node content in buildMessages ---

func TestBuildMessages_PlainTextContent(t *testing.T) {
	// Node content that is plain text (not JSON) should be wrapped as a
	// JSON string by contentToRawMessage — never panic or produce invalid JSON.
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "Hello, how are you?"},
		{NodeType: types.NodeTypeAssistant, Content: "I'm doing well, thanks!"},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Each message content should be valid JSON.
	for i, msg := range messages {
		if !json.Valid(msg.Content) {
			t.Errorf("message[%d] content is not valid JSON: %s", i, msg.Content)
		}
	}

	// Verify the text survives round-trip.
	var text string
	if err := json.Unmarshal(messages[1].Content, &text); err != nil {
		t.Fatalf("expected JSON string for assistant content: %v", err)
	}
	if text != "I'm doing well, thanks!" {
		t.Errorf("got %q, want %q", text, "I'm doing well, thanks!")
	}
}

func TestBuildMessages_UnknownBlockTypes(t *testing.T) {
	// Content that is a JSON array with unknown block types should pass
	// through without error. buildMessages is provider-agnostic.
	unknownContent := `[{"type":"unknown_fancy_block","data":"xyz"},{"type":"text","text":"normal"}]`
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: unknownContent},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// The JSON array should pass through intact.
	if string(messages[1].Content) != unknownContent {
		t.Errorf("unknown block types should pass through:\ngot:  %s\nwant: %s", messages[1].Content, unknownContent)
	}
}

func TestBuildMessages_NullAndEmptyContent(t *testing.T) {
	// Nodes with empty or whitespace-only content should not cause panics.
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: ""},
		{NodeType: types.NodeTypeAssistant, Content: "   "},
		{NodeType: types.NodeTypeUser, Content: "follow up"},
	}

	// This must not panic.
	messages := buildMessages(ancestors)

	// All three should produce messages (buildMessages doesn't filter empty content).
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Each content should be valid JSON.
	for i, msg := range messages {
		if !json.Valid(msg.Content) {
			t.Errorf("message[%d] content is not valid JSON: %s", i, msg.Content)
		}
	}
}

func TestBuildMessages_LargeContent(t *testing.T) {
	// Very large content (>1MB) should be handled without panic.
	largeText := strings.Repeat("x", 1024*1024+1) // 1MB + 1 byte
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: largeText},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Verify the large content survived.
	var text string
	if err := json.Unmarshal(messages[1].Content, &text); err != nil {
		t.Fatalf("expected JSON string for large content: %v", err)
	}
	if len(text) != 1024*1024+1 {
		t.Errorf("large content length = %d, want %d", len(text), 1024*1024+1)
	}
}

func TestBuildMessages_InvalidJSONArrayContent(t *testing.T) {
	// Content that starts with '[' but isn't valid JSON should be treated as
	// plain text (JSON-encoded string), not cause a panic.
	brokenArray := `[{"type":"text","text":"unclosed`
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: brokenArray},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// contentToRawMessage detects invalid JSON and wraps as string.
	if !json.Valid(messages[1].Content) {
		t.Errorf("message content should be valid JSON even with broken input: %s", messages[1].Content)
	}

	// The broken array should be treated as a plain text string.
	var text string
	if err := json.Unmarshal(messages[1].Content, &text); err != nil {
		t.Fatalf("expected JSON string fallback: %v", err)
	}
	if text != brokenArray {
		t.Errorf("got %q, want %q", text, brokenArray)
	}
}

func TestBuildMessages_SpecialCharsInContent(t *testing.T) {
	// Content with special characters (newlines, tabs, quotes, backslashes,
	// unicode, null bytes) should be safely encoded.
	specialContent := "line1\nline2\ttab \"quotes\" \\back null:\x00 emoji:🎉"
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: specialContent},
		{NodeType: types.NodeTypeAssistant, Content: "ok"},
	}

	messages := buildMessages(ancestors)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	if !json.Valid(messages[0].Content) {
		t.Errorf("message with special chars should produce valid JSON: %s", messages[0].Content)
	}
}

func TestContentToRawMessage_NullByte(t *testing.T) {
	// Null bytes in content should be safely encoded.
	input := "before\x00after"
	raw := contentToRawMessage(input)
	if !json.Valid(raw) {
		t.Errorf("null byte content should produce valid JSON: %s", raw)
	}
}

// --- 3d: Output group budget boundary ---

func TestOutputGroupContinuation_ExactBudgetBoundary(t *testing.T) {
	// When cumulative output tokens land exactly at the budget limit,
	// continuation should stop (cumulativeOutputToks < groupBudget is false).
	mgr, store, cleanup := newTestManagerWithSequence(t, []sequenceResponse{
		{text: "first", stopReason: "max_tokens", outputToks: 500},
		{text: " second", stopReason: "max_tokens", outputToks: 500}, // cumulative=1000, exactly at budget
		{text: " third", stopReason: "end_turn", outputToks: 100},    // should NOT be reached
	})
	defer cleanup()

	ctx := context.Background()
	// maxOutputGroupTokens=1000, so after 2 calls (500+500=1000), should stop.
	events, err := mgr.Prompt(ctx, "write", "", "", nil, nil, 500, 1000)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var allText string
	for _, ev := range evs {
		if ev.Type == types.StreamEventDelta {
			allText += ev.Content
		}
		if ev.Type == types.StreamEventError {
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}

	// Only text from first two calls.
	if allText != "first second" {
		t.Errorf("text = %q, want %q", allText, "first second")
	}

	// Verify only 2 assistant nodes created (not 3).
	var nodeID string
	for _, ev := range evs {
		if ev.Type == types.StreamEventNodeSaved {
			nodeID = ev.NodeID
		}
	}
	ancestors, err := store.GetAncestors(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	// 1 user + 2 assistant = 3 ancestors
	if len(ancestors) != 3 {
		t.Errorf("expected 3 ancestors (1 user + 2 assistant), got %d", len(ancestors))
	}
}

func TestOutputGroupContinuation_OneBelowBudget(t *testing.T) {
	// When cumulative tokens are 1 below the budget, one more continuation
	// should be attempted.
	mgr, store, cleanup := newTestManagerWithSequence(t, []sequenceResponse{
		{text: "first", stopReason: "max_tokens", outputToks: 499},
		{text: " second", stopReason: "max_tokens", outputToks: 1}, // cumulative=500, exactly at budget, but < check was before this call
		{text: " third", stopReason: "end_turn", outputToks: 100},  // reached because 499 < 500
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "write", "", "", nil, nil, 500, 500)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var allText string
	for _, ev := range evs {
		if ev.Type == types.StreamEventDelta {
			allText += ev.Content
		}
	}

	// After first call: cumulative=499 < budget=500 → continue.
	// After second call: cumulative=500, not < 500 → stop.
	// Third response is NOT reached.
	if allText != "first second" {
		t.Errorf("text = %q, want %q", allText, "first second")
	}

	var nodeID string
	for _, ev := range evs {
		if ev.Type == types.StreamEventNodeSaved {
			nodeID = ev.NodeID
		}
	}
	ancestors, err := store.GetAncestors(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ancestors) != 3 {
		t.Errorf("expected 3 ancestors (1 user + 2 assistant), got %d", len(ancestors))
	}
}

func TestOutputGroupContinuation_DefaultBudgetMultiplier(t *testing.T) {
	// When maxOutputGroupTokens=0, budget = maxTokens * 4.
	// With maxTokens=100, budget=400. First call uses 300 (< 400 → continue),
	// second uses 200 (cumulative=500 ≥ 400 → stop).
	mgr, _, cleanup := newTestManagerWithSequence(t, []sequenceResponse{
		{text: "first", stopReason: "max_tokens", outputToks: 300},
		{text: " second", stopReason: "max_tokens", outputToks: 200},
		{text: " third", stopReason: "end_turn", outputToks: 50}, // NOT reached
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "write", "", "", nil, nil, 100, 0) // budget = 100*4 = 400
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var allText string
	for _, ev := range evs {
		if ev.Type == types.StreamEventDelta {
			allText += ev.Content
		}
	}

	if allText != "first second" {
		t.Errorf("text = %q, want %q", allText, "first second")
	}
}

func TestOutputGroupContinuation_ProviderFailsDuringContinuation(t *testing.T) {
	// When the continuation provider.Stream() call fails, should emit
	// the last saved node ID as NodeSaved (graceful degradation).
	prov := &sequenceProvider{responses: []sequenceResponse{
		{text: "partial content", stopReason: "max_tokens", outputToks: 100},
		// No second response — sequenceProvider will return error.
	}}

	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	defer store.Close()

	mgr := NewManager(store, prov)

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "generate", "", "", nil, nil, 1000, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var allText string
	var gotNodeSaved bool
	var savedNodeID string
	for _, ev := range evs {
		switch ev.Type {
		case types.StreamEventDelta:
			allText += ev.Content
		case types.StreamEventNodeSaved:
			gotNodeSaved = true
			savedNodeID = ev.NodeID
		case types.StreamEventError:
			t.Errorf("should not get error event — continuation failure is graceful: %v", ev.Error)
		}
	}

	// The partial content from the first call should still be visible.
	if allText != "partial content" {
		t.Errorf("text = %q, want %q", allText, "partial content")
	}

	// The first node should be emitted as NodeSaved (graceful fallback).
	if !gotNodeSaved {
		t.Fatal("expected NodeSaved event with the last successfully saved node")
	}

	// Verify the saved node exists and has the partial content.
	node, err := store.GetNode(ctx, savedNodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node == nil {
		t.Fatal("saved node not found in DB")
	}
	if node.Content != "partial content" {
		t.Errorf("saved node content = %q, want %q", node.Content, "partial content")
	}
}

func TestOutputGroupContinuation_MaxTokensEmptyContent_NoPriorSave(t *testing.T) {
	// max_tokens with no usable content and no prior continuation →
	// should emit error event (not hang or crash).
	mgr, cleanup := newTestManager(t, mock.Config{
		Mode: "partial_max_tokens",
		// Empty FixedResponse + max_tokens stop reason → no usable content.
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil, 100, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var gotError bool
	for _, ev := range evs {
		if ev.Type == types.StreamEventError {
			gotError = true
			if ev.Error == nil || !strings.Contains(ev.Error.Error(), "max_tokens") {
				t.Errorf("expected error about max_tokens, got: %v", ev.Error)
			}
		}
		if ev.Type == types.StreamEventNodeSaved {
			t.Error("should not have NodeSaved when max_tokens with no content")
		}
	}
	if !gotError {
		t.Error("expected error event for max_tokens with no usable content")
	}
}

func TestOutputGroupContinuation_MaxTokensEmptyContent_WithPriorSave(t *testing.T) {
	// max_tokens with no usable content but prior continuation saved →
	// should emit NodeSaved with the prior node ID (not error).
	mgr, _, cleanup := newTestManagerWithSequence(t, []sequenceResponse{
		{text: "good content", stopReason: "max_tokens", outputToks: 100},
		{text: "", stopReason: "max_tokens", outputToks: 0}, // empty continuation
	})
	defer cleanup()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "write", "", "", nil, nil, 1000, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	evs := drainEvents(t, events, 5*time.Second)

	var gotNodeSaved bool
	var gotError bool
	for _, ev := range evs {
		switch ev.Type {
		case types.StreamEventNodeSaved:
			gotNodeSaved = true
		case types.StreamEventError:
			gotError = true
		}
	}

	// Should emit NodeSaved (the prior saved node), not error.
	if !gotNodeSaved {
		t.Error("expected NodeSaved with prior continuation node")
	}
	if gotError {
		t.Error("should not get error when prior continuation exists")
	}
}
