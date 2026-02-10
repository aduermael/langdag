package sqlite

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/langdag/langdag/pkg/types"
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
