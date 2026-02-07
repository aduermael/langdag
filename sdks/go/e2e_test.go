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

func TestE2E_Chat_NonStreaming(t *testing.T) {
	url := e2eURL(t)
	c := NewClient(url, WithTimeout(30*time.Second))
	ctx := context.Background()

	// Start a new chat
	resp, err := c.Chat(ctx, &NewChatRequest{
		Message: "Hello, this is a test message",
	}, nil)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if resp.DAGID == "" {
		t.Error("expected non-empty DAGID")
	}
	if resp.NodeID == "" {
		t.Error("expected non-empty NodeID")
	}
	if resp.Content == "" {
		t.Error("expected non-empty content in response")
	}

	// Continue the conversation
	resp2, err := c.ContinueChat(ctx, resp.DAGID, &ContinueChatRequest{
		Message: "Follow up message",
	}, nil)
	if err != nil {
		t.Fatalf("continue chat failed: %v", err)
	}
	if resp2.DAGID != resp.DAGID {
		t.Errorf("expected same DAGID %s, got %s", resp.DAGID, resp2.DAGID)
	}
	if resp2.Content == "" {
		t.Error("expected non-empty content in continue response")
	}

	// Get the DAG
	dag, err := c.GetDAG(ctx, resp.DAGID)
	if err != nil {
		t.Fatalf("get DAG failed: %v", err)
	}
	if dag.ID != resp.DAGID {
		t.Errorf("expected DAG ID %s, got %s", resp.DAGID, dag.ID)
	}
	// Should have at least 4 nodes: user1, assistant1, user2, assistant2
	if len(dag.Nodes) < 4 {
		t.Errorf("expected at least 4 nodes, got %d", len(dag.Nodes))
	}

	// List DAGs
	dags, err := c.ListDAGs(ctx)
	if err != nil {
		t.Fatalf("list DAGs failed: %v", err)
	}
	found := false
	for _, d := range dags {
		if d.ID == resp.DAGID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DAG %s not found in list", resp.DAGID)
	}

	// Delete the DAG
	delResp, err := c.DeleteDAG(ctx, resp.DAGID)
	if err != nil {
		t.Fatalf("delete DAG failed: %v", err)
	}
	if delResp.Status != "deleted" {
		t.Errorf("expected status deleted, got %s", delResp.Status)
	}
}

func TestE2E_Chat_Streaming(t *testing.T) {
	url := e2eURL(t)
	c := NewClient(url, WithTimeout(30*time.Second))
	ctx := context.Background()

	var dagID string
	var nodeID string
	var content strings.Builder
	eventTypes := make(map[SSEEventType]int)

	err := c.ChatStream(ctx, &NewChatRequest{
		Message: "Tell me something interesting",
	}, func(e SSEEvent) error {
		eventTypes[e.Type]++
		switch e.Type {
		case SSEEventStart:
			dagID = e.DAGID
		case SSEEventDelta:
			content.WriteString(e.Content)
		case SSEEventDone:
			nodeID = e.NodeID
		}
		return nil
	})
	if err != nil {
		t.Fatalf("streaming chat failed: %v", err)
	}

	if dagID == "" {
		t.Error("expected non-empty DAGID from start event")
	}
	if nodeID == "" {
		t.Error("expected non-empty NodeID from done event")
	}
	if content.Len() == 0 {
		t.Error("expected non-empty streamed content")
	}
	if eventTypes[SSEEventStart] != 1 {
		t.Errorf("expected 1 start event, got %d", eventTypes[SSEEventStart])
	}
	if eventTypes[SSEEventDelta] < 1 {
		t.Errorf("expected at least 1 delta event, got %d", eventTypes[SSEEventDelta])
	}
	if eventTypes[SSEEventDone] != 1 {
		t.Errorf("expected 1 done event, got %d", eventTypes[SSEEventDone])
	}

	// Clean up
	if dagID != "" {
		c.DeleteDAG(ctx, dagID)
	}
}

func TestE2E_ForkChat(t *testing.T) {
	url := e2eURL(t)
	c := NewClient(url, WithTimeout(30*time.Second))
	ctx := context.Background()

	// Start a conversation
	resp, err := c.Chat(ctx, &NewChatRequest{
		Message: "First message",
	}, nil)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	// Fork from the assistant node
	forkResp, err := c.ForkChat(ctx, resp.DAGID, &ForkChatRequest{
		NodeID:  resp.NodeID,
		Message: "Alternative follow-up",
	}, nil)
	if err != nil {
		t.Fatalf("fork chat failed: %v", err)
	}
	if forkResp.DAGID == "" {
		t.Error("expected non-empty DAGID from fork")
	}
	if forkResp.Content == "" {
		t.Error("expected non-empty content from fork")
	}

	// Clean up
	c.DeleteDAG(ctx, resp.DAGID)
	if forkResp.DAGID != resp.DAGID {
		c.DeleteDAG(ctx, forkResp.DAGID)
	}
}
