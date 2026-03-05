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
	events, err := mgr.Prompt(ctx, "hello", "", "", nil)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Must complete within 5 seconds (should be nearly instant).
	_ = drainEvents(t, events, 5*time.Second)
}

func TestBuildMessages_MergesConsecutiveUserRoles(t *testing.T) {
	// If ancestors contain consecutive user-role nodes (e.g. user + tool_result),
	// buildMessages should merge them into a single user message.
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "What's the weather?"},
		{NodeType: types.NodeTypeAssistant, Content: `[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"loc":"NYC"}}]`},
		// Two consecutive user-role nodes (tool_result followed by tool_result):
		{NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"toolu_1","content":"Sunny"}]`},
	}

	messages := buildMessages(ancestors)

	// Should have 3 messages: user, assistant, user
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// All roles should alternate
	expectedRoles := []string{"user", "assistant", "user"}
	for i, msg := range messages {
		if msg.Role != expectedRoles[i] {
			t.Errorf("message[%d].Role = %q, want %q", i, msg.Role, expectedRoles[i])
		}
	}

	// The assistant message should be a JSON array (content blocks passed through)
	if messages[1].Content[0] != '[' {
		t.Errorf("assistant message should be JSON array, got: %s", messages[1].Content)
	}
}

func TestBuildMessages_MergesConsecutiveUserWithAppend(t *testing.T) {
	// Simulates what happens when PromptFrom is called on a tool_result node:
	// ancestors end with tool_result (user role), then we append another user message.
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "Start"},
		{NodeType: types.NodeTypeAssistant, Content: `[{"type":"text","text":"Using tool"},{"type":"tool_use","id":"t1","name":"search","input":{}}]`},
		{NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"t1","content":"result"}]`},
		// This second tool_result simulates another user-role node
		{NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"t2","content":"result2"}]`},
	}

	messages := buildMessages(ancestors)

	// The two tool_result nodes should be merged into one user message.
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages (user, assistant, merged user), got %d", len(messages))
	}

	// Verify the merged user message is a JSON array containing both tool_results
	var blocks []json.RawMessage
	if err := json.Unmarshal(messages[2].Content, &blocks); err != nil {
		t.Fatalf("merged content is not a JSON array: %v\ncontent: %s", err, messages[2].Content)
	}
	if len(blocks) != 2 {
		t.Errorf("expected 2 content blocks in merged message, got %d", len(blocks))
	}
}

func TestBuildMessages_SkipsToolCallNodes(t *testing.T) {
	// NodeTypeToolCall nodes should be skipped (they're metadata from LangGraph imports).
	ancestors := []*types.Node{
		{NodeType: types.NodeTypeUser, Content: "hello"},
		{NodeType: types.NodeTypeAssistant, Content: "I'll search."},
		{NodeType: types.NodeTypeToolCall, Content: `{"name":"search","input":{}}`},
		{NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"t1","content":"found"}]`},
	}

	messages := buildMessages(ancestors)

	// tool_call skipped, tool_result maps to user → but would be consecutive
	// with previous... wait, assistant is before tool_call, tool_call is skipped,
	// so tool_result (user) follows assistant. That's fine.
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
	// Should be passed through as-is
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
}

func (f *failingStorage) Init(ctx context.Context) error                                 { return f.inner.Init(ctx) }
func (f *failingStorage) Close() error                                                   { return f.inner.Close() }
func (f *failingStorage) GetNode(ctx context.Context, id string) (*types.Node, error)    { return f.inner.GetNode(ctx, id) }
func (f *failingStorage) GetNodeByPrefix(ctx context.Context, p string) (*types.Node, error) { return f.inner.GetNodeByPrefix(ctx, p) }
func (f *failingStorage) GetNodeChildren(ctx context.Context, p string) ([]*types.Node, error) { return f.inner.GetNodeChildren(ctx, p) }
func (f *failingStorage) GetSubtree(ctx context.Context, id string) ([]*types.Node, error) { return f.inner.GetSubtree(ctx, id) }
func (f *failingStorage) GetAncestors(ctx context.Context, id string) ([]*types.Node, error) { return f.inner.GetAncestors(ctx, id) }
func (f *failingStorage) ListRootNodes(ctx context.Context) ([]*types.Node, error)       { return f.inner.ListRootNodes(ctx) }
func (f *failingStorage) UpdateNode(ctx context.Context, node *types.Node) error          { return f.inner.UpdateNode(ctx, node) }
func (f *failingStorage) DeleteNode(ctx context.Context, id string) error                 { return f.inner.DeleteNode(ctx, id) }
func (f *failingStorage) CreateAlias(ctx context.Context, n, a string) error              { return f.inner.CreateAlias(ctx, n, a) }
func (f *failingStorage) DeleteAlias(ctx context.Context, a string) error                 { return f.inner.DeleteAlias(ctx, a) }
func (f *failingStorage) GetNodeByAlias(ctx context.Context, a string) (*types.Node, error) { return f.inner.GetNodeByAlias(ctx, a) }
func (f *failingStorage) ListAliases(ctx context.Context, id string) ([]string, error)   { return f.inner.ListAliases(ctx, id) }

func (f *failingStorage) CreateNode(ctx context.Context, node *types.Node) error {
	f.calls++
	if f.calls > f.failAfter {
		return fmt.Errorf("injected storage failure")
	}
	return f.inner.CreateNode(ctx, node)
}

func TestStreamResponse_CreateNodeFailure_DoesNotHang(t *testing.T) {
	// When CreateNode fails for the assistant node, the stream must send an
	// error event and close — never hang waiting for NodeSaved.
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}

	// Fail after 1 successful CreateNode (the user node), so the assistant
	// node save fails.
	fs := &failingStorage{inner: store, failAfter: 1}
	prov := mock.New(mock.Config{Mode: "fixed", FixedResponse: "test response"})
	mgr := NewManager(fs, prov)
	defer store.Close()

	ctx := context.Background()
	events, err := mgr.Prompt(ctx, "hello", "", "", nil)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Must complete within 5 seconds — the bug would cause an infinite hang.
	allEvents := drainEvents(t, events, 5*time.Second)

	// Should have received an error event
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

	// Should NOT have a NodeSaved event
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventNodeSaved {
			t.Error("should not have NodeSaved event when CreateNode fails")
		}
	}
}

func TestPromptFrom_ToolResultParent_RolesMerged(t *testing.T) {
	// When PromptFrom is called on a node that maps to "user" role,
	// the messages should still alternate correctly (merged into one user msg).
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
	events1, err := mgr.Prompt(ctx, "Search for test", "", "", nil)
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
	events2, err := mgr.PromptFrom(ctx, assistantNodeID, toolResult, "", nil)
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

	// Turn 3: send plain text continuing from that assistant node.
	// This should NOT cause consecutive user messages since parent is assistant.
	events3, err := mgr.PromptFrom(ctx, secondNodeID, "What did you find?", "", nil)
	if err != nil {
		t.Fatalf("PromptFrom (turn 3): %v", err)
	}
	allEvents := drainEvents(t, events3, 5*time.Second)

	// Should complete without error
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventError {
			t.Errorf("unexpected error event: %v", ev.Error)
		}
	}
}
