package conversation

import (
	"context"
	"encoding/json"
	"fmt"
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
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil)
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
	events, err := mgr.PromptFrom(ctx, "a1", "Actually, nevermind", "", nil, nil)
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
	events, err := mgr.PromptFrom(ctx, "a1", toolResult, "", nil, nil)
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
	events, err := mgr.PromptFrom(ctx, "a1", toolResult, "", nil, nil)
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
	events, err := mgr.Prompt(ctx, "hello", "", "", nil, nil)
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
	events, err := mgr.Prompt(ctx, "find it", "", "", nil, nil)
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
	events1, err := mgr.Prompt(ctx, "What's the weather?", "", "", nil, nil)
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
	events2, err := mgr.PromptFrom(ctx, assistantNodeID, "Actually, never mind", "", nil, nil)
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
	events1, err := mgr.Prompt(ctx, "Search for test", "", "", nil, nil)
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
	events2, err := mgr.PromptFrom(ctx, assistantNodeID, toolResult, "", nil, nil)
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
	events3, err := mgr.PromptFrom(ctx, secondNodeID, "What did you find?", "", nil, nil)
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
