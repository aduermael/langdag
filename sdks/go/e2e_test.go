// E2E tests that connect to a running LangDAG server with mock provider.
// Run with: LANGDAG_E2E_URL=http://localhost:8080 go test -v -run TestE2E ./...
// The server must be started with LANGDAG_PROVIDER=mock.

package langdag

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func e2eURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("LANGDAG_E2E_URL")
	if url == "" {
		t.Skip("LANGDAG_E2E_URL not set, skipping E2E test")
	}
	return url
}

func TestE2E_Health(t *testing.T) {
	url := e2eURL(t)
	c := NewClient(url, WithTimeout(10*time.Second))

	resp, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
}

func TestE2E_Prompt(t *testing.T) {
	url := e2eURL(t)
	c := NewClient(url, WithTimeout(30*time.Second))
	ctx := context.Background()

	// Start a new conversation
	node, err := c.Prompt(ctx, "Hello, this is a test message")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if node.ID == "" {
		t.Error("expected non-empty node ID")
	}
	if node.Content == "" {
		t.Error("expected non-empty content")
	}

	// Continue from the assistant node
	node2, err := node.Prompt(ctx, "Follow up message")
	if err != nil {
		t.Fatalf("continue prompt failed: %v", err)
	}
	if node2.ID == "" || node2.ID == node.ID {
		t.Error("expected different node ID for continuation")
	}
	if node2.Content == "" {
		t.Error("expected non-empty content in continue response")
	}

	// List roots
	roots, err := c.ListRoots(ctx)
	if err != nil {
		t.Fatalf("list roots failed: %v", err)
	}
	found := false
	var rootID string
	for _, r := range roots {
		// The root is the user node (parent of the first assistant node)
		// Find it by checking recent roots
		if r.Title != "" {
			found = true
			rootID = r.ID
			break
		}
	}
	if !found {
		t.Error("conversation root not found in list")
	}

	// Get tree
	if rootID != "" {
		tree, err := c.GetTree(ctx, rootID)
		if err != nil {
			t.Fatalf("get tree failed: %v", err)
		}
		// Should have at least root(user) + assistant + user + assistant = 4 nodes
		if len(tree.Nodes) < 4 {
			t.Errorf("expected at least 4 nodes, got %d", len(tree.Nodes))
		}
	}

	// Delete
	if rootID != "" {
		err = c.DeleteNode(ctx, rootID)
		if err != nil {
			t.Fatalf("delete node failed: %v", err)
		}
	}
}

func TestE2E_PromptStream(t *testing.T) {
	url := e2eURL(t)
	c := NewClient(url, WithTimeout(30*time.Second))
	ctx := context.Background()

	stream, err := c.PromptStream(ctx, "Tell me something interesting")
	if err != nil {
		t.Fatalf("stream prompt failed: %v", err)
	}

	var content strings.Builder
	eventTypes := make(map[string]int)

	for event := range stream.Events() {
		eventTypes[event.Type]++
		if event.Type == "delta" {
			content.WriteString(event.Content)
		}
	}

	node, err := stream.Node()
	if err != nil {
		t.Fatalf("stream.Node() failed: %v", err)
	}
	if node.ID == "" {
		t.Error("expected non-empty node ID")
	}
	if content.Len() == 0 {
		t.Error("expected non-empty streamed content")
	}
	if eventTypes["start"] != 1 {
		t.Errorf("expected 1 start event, got %d", eventTypes["start"])
	}
	if eventTypes["delta"] < 1 {
		t.Errorf("expected at least 1 delta event, got %d", eventTypes["delta"])
	}
	if eventTypes["done"] != 1 {
		t.Errorf("expected 1 done event, got %d", eventTypes["done"])
	}

	// Clean up: find root and delete
	roots, _ := c.ListRoots(ctx)
	for _, r := range roots {
		c.DeleteNode(ctx, r.ID)
	}
}

func TestE2E_Branch(t *testing.T) {
	url := e2eURL(t)
	c := NewClient(url, WithTimeout(30*time.Second))
	ctx := context.Background()

	// Start a conversation
	node, err := c.Prompt(ctx, "First message")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	// Branch from the same node twice
	branch1, err := node.Prompt(ctx, "Alternative A")
	if err != nil {
		t.Fatalf("branch A failed: %v", err)
	}

	branch2, err := node.Prompt(ctx, "Alternative B")
	if err != nil {
		t.Fatalf("branch B failed: %v", err)
	}

	if branch1.ID == branch2.ID {
		t.Error("expected different node IDs for branches")
	}

	// Clean up
	roots, _ := c.ListRoots(ctx)
	for _, r := range roots {
		c.DeleteNode(ctx, r.ID)
	}
}
