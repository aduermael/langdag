package sqlite

import (
	"context"
	"os"
	"testing"
	"time"

	"langdag.com/langdag/types"
)

func setupTestDB(t *testing.T) *SQLiteStorage {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "langdag-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestCreateAndGetNode(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	node := &types.Node{
		ID:           "node-1",
		Sequence:     0,
		NodeType:     types.NodeTypeUser,
		Content:      "Hello, world!",
		Model:        "claude-sonnet-4-20250514",
		Status:       "completed",
		Title:        "Test conversation",
		SystemPrompt: "You are helpful.",
		CreatedAt:    time.Now(),
	}

	if err := store.CreateNode(ctx, node); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	got, err := store.GetNode(ctx, "node-1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got == nil {
		t.Fatal("GetNode: returned nil")
	}
	if got.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", got.Content, "Hello, world!")
	}
	if got.Title != "Test conversation" {
		t.Errorf("Title = %q, want %q", got.Title, "Test conversation")
	}
	if got.SystemPrompt != "You are helpful." {
		t.Errorf("SystemPrompt = %q, want %q", got.SystemPrompt, "You are helpful.")
	}
	if got.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-sonnet-4-20250514")
	}
}

func TestGetNodeNotFound(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	got, err := store.GetNode(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got != nil {
		t.Error("GetNode: expected nil for nonexistent ID")
	}
}

func TestGetNodeByPrefix(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	node := &types.Node{
		ID:        "abcdef-1234-5678",
		Sequence:  0,
		NodeType:  types.NodeTypeUser,
		Content:   "prefix test",
		CreatedAt: time.Now(),
	}
	if err := store.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetNodeByPrefix(ctx, "abcdef")
	if err != nil {
		t.Fatalf("GetNodeByPrefix: %v", err)
	}
	if got == nil {
		t.Fatal("GetNodeByPrefix: returned nil")
	}
	if got.ID != "abcdef-1234-5678" {
		t.Errorf("ID = %q, want %q", got.ID, "abcdef-1234-5678")
	}
}

func TestListRootNodes(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create root node
	root := &types.Node{
		ID:        "root-1",
		Sequence:  0,
		NodeType:  types.NodeTypeUser,
		Content:   "root message",
		Title:     "Root",
		CreatedAt: time.Now(),
	}
	if err := store.CreateNode(ctx, root); err != nil {
		t.Fatal(err)
	}

	// Create child node
	child := &types.Node{
		ID:        "child-1",
		ParentID:  "root-1",
		Sequence:  1,
		NodeType:  types.NodeTypeAssistant,
		Content:   "child response",
		CreatedAt: time.Now(),
	}
	if err := store.CreateNode(ctx, child); err != nil {
		t.Fatal(err)
	}

	roots, err := store.ListRootNodes(ctx)
	if err != nil {
		t.Fatalf("ListRootNodes: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("ListRootNodes: got %d roots, want 1", len(roots))
	}
	if roots[0].ID != "root-1" {
		t.Errorf("root ID = %q, want %q", roots[0].ID, "root-1")
	}
}

func TestGetNodeChildren(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	root := &types.Node{ID: "root", Sequence: 0, NodeType: types.NodeTypeUser, Content: "root", CreatedAt: time.Now()}
	child1 := &types.Node{ID: "child-1", ParentID: "root", Sequence: 1, NodeType: types.NodeTypeAssistant, Content: "c1", CreatedAt: time.Now()}
	child2 := &types.Node{ID: "child-2", ParentID: "root", Sequence: 2, NodeType: types.NodeTypeUser, Content: "c2", CreatedAt: time.Now()}

	for _, n := range []*types.Node{root, child1, child2} {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	children, err := store.GetNodeChildren(ctx, "root")
	if err != nil {
		t.Fatalf("GetNodeChildren: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("got %d children, want 2", len(children))
	}
	if children[0].ID != "child-1" || children[1].ID != "child-2" {
		t.Errorf("children = [%s, %s], want [child-1, child-2]", children[0].ID, children[1].ID)
	}
}

func TestGetSubtree(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Build a small tree: root → child → grandchild
	nodes := []*types.Node{
		{ID: "root", Sequence: 0, NodeType: types.NodeTypeUser, Content: "root", CreatedAt: time.Now()},
		{ID: "child", ParentID: "root", Sequence: 1, NodeType: types.NodeTypeAssistant, Content: "child", CreatedAt: time.Now()},
		{ID: "grandchild", ParentID: "child", Sequence: 2, NodeType: types.NodeTypeUser, Content: "grandchild", CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	subtree, err := store.GetSubtree(ctx, "root")
	if err != nil {
		t.Fatalf("GetSubtree: %v", err)
	}
	if len(subtree) != 3 {
		t.Fatalf("got %d nodes, want 3", len(subtree))
	}

	// Subtree from child should only include child and grandchild
	subtree2, err := store.GetSubtree(ctx, "child")
	if err != nil {
		t.Fatalf("GetSubtree from child: %v", err)
	}
	if len(subtree2) != 2 {
		t.Fatalf("got %d nodes, want 2", len(subtree2))
	}
}

func TestGetAncestors(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	nodes := []*types.Node{
		{ID: "root", Sequence: 0, NodeType: types.NodeTypeUser, Content: "root", SystemPrompt: "be helpful", CreatedAt: time.Now()},
		{ID: "child", ParentID: "root", Sequence: 1, NodeType: types.NodeTypeAssistant, Content: "child", CreatedAt: time.Now()},
		{ID: "grandchild", ParentID: "child", Sequence: 2, NodeType: types.NodeTypeUser, Content: "grandchild", CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	ancestors, err := store.GetAncestors(ctx, "grandchild")
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	if len(ancestors) != 3 {
		t.Fatalf("got %d ancestors, want 3", len(ancestors))
	}
	// Should be ordered root-first
	if ancestors[0].ID != "root" {
		t.Errorf("first ancestor = %q, want %q", ancestors[0].ID, "root")
	}
	if ancestors[0].SystemPrompt != "be helpful" {
		t.Errorf("root SystemPrompt = %q, want %q", ancestors[0].SystemPrompt, "be helpful")
	}
	if ancestors[2].ID != "grandchild" {
		t.Errorf("last ancestor = %q, want %q", ancestors[2].ID, "grandchild")
	}
}

func TestUpdateNode(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	node := &types.Node{
		ID:        "update-test",
		Sequence:  0,
		NodeType:  types.NodeTypeUser,
		Content:   "original",
		Title:     "Original Title",
		CreatedAt: time.Now(),
	}
	if err := store.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}

	node.Title = "Updated Title"
	node.Content = "updated content"
	node.Status = "completed"
	if err := store.UpdateNode(ctx, node); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}

	got, err := store.GetNode(ctx, "update-test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q", got.Title, "Updated Title")
	}
	if got.Content != "updated content" {
		t.Errorf("Content = %q, want %q", got.Content, "updated content")
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want %q", got.Status, "completed")
	}
}

func TestDeleteNode(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Build tree: root → child → grandchild
	nodes := []*types.Node{
		{ID: "root", Sequence: 0, NodeType: types.NodeTypeUser, Content: "root", CreatedAt: time.Now()},
		{ID: "child", ParentID: "root", Sequence: 1, NodeType: types.NodeTypeAssistant, Content: "child", CreatedAt: time.Now()},
		{ID: "grandchild", ParentID: "child", Sequence: 2, NodeType: types.NodeTypeUser, Content: "grandchild", CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	// Delete root — should cascade to all descendants
	if err := store.DeleteNode(ctx, "root"); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}

	for _, id := range []string{"root", "child", "grandchild"} {
		got, err := store.GetNode(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Errorf("node %q still exists after cascade delete", id)
		}
	}
}

func TestAliasCreateAndResolve(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	node := &types.Node{
		ID:        "alias-node-1",
		Sequence:  0,
		NodeType:  types.NodeTypeUser,
		Content:   "alias test",
		CreatedAt: time.Now(),
	}
	if err := store.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}

	// Create alias
	if err := store.CreateAlias(ctx, "alias-node-1", "my-bookmark"); err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}

	// Resolve by alias
	got, err := store.GetNodeByAlias(ctx, "my-bookmark")
	if err != nil {
		t.Fatalf("GetNodeByAlias: %v", err)
	}
	if got == nil {
		t.Fatal("GetNodeByAlias: returned nil")
	}
	if got.ID != "alias-node-1" {
		t.Errorf("ID = %q, want %q", got.ID, "alias-node-1")
	}

	// List aliases
	aliases, err := store.ListAliases(ctx, "alias-node-1")
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(aliases) != 1 || aliases[0] != "my-bookmark" {
		t.Errorf("aliases = %v, want [my-bookmark]", aliases)
	}

	// Delete alias
	if err := store.DeleteAlias(ctx, "my-bookmark"); err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}

	got, err = store.GetNodeByAlias(ctx, "my-bookmark")
	if err != nil {
		t.Fatalf("GetNodeByAlias after delete: %v", err)
	}
	if got != nil {
		t.Error("GetNodeByAlias: expected nil after delete")
	}
}

func TestAliasMultiple(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	node := &types.Node{
		ID:        "multi-alias-node",
		Sequence:  0,
		NodeType:  types.NodeTypeUser,
		Content:   "multi alias test",
		CreatedAt: time.Now(),
	}
	if err := store.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}

	for _, alias := range []string{"alpha", "beta", "gamma"} {
		if err := store.CreateAlias(ctx, "multi-alias-node", alias); err != nil {
			t.Fatalf("CreateAlias(%s): %v", alias, err)
		}
	}

	aliases, err := store.ListAliases(ctx, "multi-alias-node")
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(aliases) != 3 {
		t.Fatalf("got %d aliases, want 3", len(aliases))
	}
}

func TestAliasCascadeOnNodeDelete(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	node := &types.Node{
		ID:        "cascade-node",
		Sequence:  0,
		NodeType:  types.NodeTypeUser,
		Content:   "cascade test",
		CreatedAt: time.Now(),
	}
	if err := store.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAlias(ctx, "cascade-node", "will-be-deleted"); err != nil {
		t.Fatal(err)
	}

	// Delete the node — alias should cascade
	if err := store.DeleteNode(ctx, "cascade-node"); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetNodeByAlias(ctx, "will-be-deleted")
	if err != nil {
		t.Fatalf("GetNodeByAlias after cascade: %v", err)
	}
	if got != nil {
		t.Error("alias still resolves after node deletion")
	}
}

// --- Tool ID index tests ---

func TestIndexToolIDs_AndGetOrphaned(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create nodes.
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

	// Index tool_use ID.
	if err := store.IndexToolIDs(ctx, "a1", []string{"t1"}, "use"); err != nil {
		t.Fatal(err)
	}

	// Should be orphaned (no result).
	orphans, err := store.GetOrphanedToolUses(ctx, []string{"u1", "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans["a1"][0] != "t1" {
		t.Errorf("expected orphan t1 on a1, got: %v", orphans)
	}

	// Now add a tool_result.
	trNode := &types.Node{ID: "tr1", ParentID: "a1", RootID: "u1", Sequence: 2,
		NodeType: types.NodeTypeToolResult, Content: `[{"type":"tool_result","tool_use_id":"t1","content":"done"}]`, CreatedAt: time.Now()}
	if err := store.CreateNode(ctx, trNode); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexToolIDs(ctx, "tr1", []string{"t1"}, "result"); err != nil {
		t.Fatal(err)
	}

	// Should no longer be orphaned.
	orphans, err = store.GetOrphanedToolUses(ctx, []string{"u1", "a1", "tr1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected no orphans, got: %v", orphans)
	}
}

func TestIndexToolIDs_EmptyList(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	// Should be a no-op, not an error.
	if err := store.IndexToolIDs(ctx, "any", nil, "use"); err != nil {
		t.Errorf("IndexToolIDs with empty list: %v", err)
	}
}

func TestGetOrphanedToolUses_EmptyAncestors(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	orphans, err := store.GetOrphanedToolUses(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if orphans != nil {
		t.Errorf("expected nil for empty ancestors, got: %v", orphans)
	}
}

func TestIndexToolIDs_DuplicateIsIdempotent(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	node := &types.Node{ID: "a1", Sequence: 0, NodeType: types.NodeTypeAssistant, Content: "test", CreatedAt: time.Now()}
	if err := store.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}

	// Index same ID twice — should not error (INSERT OR IGNORE).
	if err := store.IndexToolIDs(ctx, "a1", []string{"t1"}, "use"); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexToolIDs(ctx, "a1", []string{"t1"}, "use"); err != nil {
		t.Errorf("duplicate IndexToolIDs should not error: %v", err)
	}
}

func TestBackfillMigration_IndexesExistingNodes(t *testing.T) {
	// Simulate upgrading from schema version 6 → 7.
	// The migration backfill should index tool IDs from existing node content.
	tmpFile, err := os.CreateTemp("", "langdag-backfill-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Run only first 6 migrations by Init, then manually insert data.
	if err := store.Init(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}

	// Insert nodes with tool_use/tool_result content.
	nodes := []*types.Node{
		{ID: "u1", RootID: "u1", Sequence: 0, NodeType: types.NodeTypeUser, Content: "hi", CreatedAt: time.Now()},
		{ID: "a1", ParentID: "u1", RootID: "u1", Sequence: 1, NodeType: types.NodeTypeAssistant,
			Content: `[{"type":"tool_use","id":"backfill_t1","name":"x","input":{}}]`, CreatedAt: time.Now()},
		{ID: "tr1", ParentID: "a1", RootID: "u1", Sequence: 2, NodeType: types.NodeTypeToolResult,
			Content: `[{"type":"tool_result","tool_use_id":"backfill_t1","content":"done"}]`, CreatedAt: time.Now()},
		// Orphaned tool_use (no result).
		{ID: "a2", ParentID: "tr1", RootID: "u1", Sequence: 3, NodeType: types.NodeTypeAssistant,
			Content: `[{"type":"tool_use","id":"backfill_t2","name":"y","input":{}}]`, CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			store.Close()
			t.Fatal(err)
		}
	}

	// Simulate downgrade: clear the tool index and reset version to 6.
	store.db.ExecContext(ctx, "DELETE FROM node_tool_ids")
	store.db.ExecContext(ctx, "UPDATE schema_version SET version = 6")
	store.Close()

	// Re-open and Init → should run migration 7 with backfill.
	store2, err := New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	if err := store2.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// backfill_t1 has a result → not orphaned.
	// backfill_t2 has no result → orphaned.
	orphans, err := store2.GetOrphanedToolUses(ctx, []string{"u1", "a1", "tr1", "a2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan after backfill, got %d: %v", len(orphans), orphans)
	}
	if orphans["a2"][0] != "backfill_t2" {
		t.Errorf("expected orphan backfill_t2, got: %v", orphans)
	}
}

func TestDeleteNodePartialSubtree(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// root → child1, root → child2
	nodes := []*types.Node{
		{ID: "root", Sequence: 0, NodeType: types.NodeTypeUser, Content: "root", CreatedAt: time.Now()},
		{ID: "child1", ParentID: "root", Sequence: 1, NodeType: types.NodeTypeAssistant, Content: "c1", CreatedAt: time.Now()},
		{ID: "child2", ParentID: "root", Sequence: 2, NodeType: types.NodeTypeUser, Content: "c2", CreatedAt: time.Now()},
	}
	for _, n := range nodes {
		if err := store.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	// Delete child1 only
	if err := store.DeleteNode(ctx, "child1"); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetNode(ctx, "child1")
	if got != nil {
		t.Error("child1 still exists")
	}

	// root and child2 should still exist
	got, _ = store.GetNode(ctx, "root")
	if got == nil {
		t.Error("root was deleted")
	}
	got, _ = store.GetNode(ctx, "child2")
	if got == nil {
		t.Error("child2 was deleted")
	}
}
