// Cross-SDK consistency tests — verify the Go SDK parses SSE fixtures
// identically to the Python and TypeScript SDKs. The exact same SSE byte
// strings are used in sdks/python/tests/test_cross_sdk.py and
// sdks/typescript/src/cross-sdk.test.ts.

package langdag

import (
	"io"
	"strings"
	"testing"
)

// Canonical SSE fixtures — byte-for-byte identical across all 3 SDKs.
const (
	// Normal happy path: start → delta → delta → done
	fixtureNormal = "event: start\ndata: {}\n\n" +
		"event: delta\ndata: {\"content\":\"Hello \"}\n\n" +
		"event: delta\ndata: {\"content\":\"world!\"}\n\n" +
		"event: done\ndata: {\"node_id\":\"test-node-1\"}\n\n"

	// Error mid-stream: start → delta → error
	fixtureErrorMidStream = "event: start\ndata: {}\n\n" +
		"event: delta\ndata: {\"content\":\"partial \"}\n\n" +
		"event: error\ndata: provider crashed\n\n"

	// Multi-line error: start → error with 3 data lines
	fixtureMultiLineError = "event: start\ndata: {}\n\n" +
		"event: error\ndata: line one\ndata: line two\ndata: line three\n\n"

	// Error-only: no start, no content
	fixtureErrorOnly = "event: error\ndata: unauthorized\n\n"
)

func streamFromFixture(fixture string) *Stream {
	return newStream(io.NopCloser(strings.NewReader(fixture)), nil)
}

func drainEvents(s *Stream) []SSEEvent {
	var events []SSEEvent
	for e := range s.Events() {
		events = append(events, e)
	}
	return events
}

func TestCrossSDK_NormalFlow(t *testing.T) {
	s := streamFromFixture(fixtureNormal)
	events := drainEvents(s)

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	if events[0].Type != "start" {
		t.Errorf("event 0: expected start, got %s", events[0].Type)
	}
	if events[1].Type != "delta" || events[1].Content != "Hello " {
		t.Errorf("event 1: expected delta 'Hello ', got %s %q", events[1].Type, events[1].Content)
	}
	if events[2].Type != "delta" || events[2].Content != "world!" {
		t.Errorf("event 2: expected delta 'world!', got %s %q", events[2].Type, events[2].Content)
	}
	if events[3].Type != "done" || events[3].NodeID != "test-node-1" {
		t.Errorf("event 3: expected done/test-node-1, got %s/%s", events[3].Type, events[3].NodeID)
	}

	if s.Content() != "Hello world!" {
		t.Errorf("content: expected 'Hello world!', got %q", s.Content())
	}
	if s.Err() != nil {
		t.Errorf("expected no error, got %v", s.Err())
	}

	node, err := s.Node()
	if err != nil {
		t.Fatalf("Node() error: %v", err)
	}
	if node.ID != "test-node-1" {
		t.Errorf("expected node test-node-1, got %s", node.ID)
	}
}

func TestCrossSDK_ErrorMidStream(t *testing.T) {
	s := streamFromFixture(fixtureErrorMidStream)
	events := drainEvents(s)

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Type != "start" {
		t.Errorf("event 0: expected start, got %s", events[0].Type)
	}
	if events[1].Type != "delta" || events[1].Content != "partial " {
		t.Errorf("event 1: expected delta 'partial ', got %s %q", events[1].Type, events[1].Content)
	}
	if events[2].Type != "error" || events[2].Error != "provider crashed" {
		t.Errorf("event 2: expected error 'provider crashed', got %s %q", events[2].Type, events[2].Error)
	}

	// Content accumulated before error must be preserved
	if s.Content() != "partial " {
		t.Errorf("content: expected 'partial ', got %q", s.Content())
	}

	// Err() returns StreamError with the error message
	err := s.Err()
	if err == nil {
		t.Fatal("expected error from Err()")
	}
	streamErr, ok := err.(*StreamError)
	if !ok {
		t.Fatalf("expected *StreamError, got %T", err)
	}
	if streamErr.Message != "provider crashed" {
		t.Errorf("expected 'provider crashed', got %q", streamErr.Message)
	}

	// Node() should fail (no done event)
	_, err = s.Node()
	if err == nil {
		t.Error("expected error from Node()")
	}
}

func TestCrossSDK_MultiLineError(t *testing.T) {
	s := streamFromFixture(fixtureMultiLineError)
	events := drainEvents(s)

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "start" {
		t.Errorf("event 0: expected start, got %s", events[0].Type)
	}

	// Multi-line data: lines joined with \n
	expectedMsg := "line one\nline two\nline three"
	if events[1].Type != "error" || events[1].Error != expectedMsg {
		t.Errorf("event 1: expected error %q, got %s %q", expectedMsg, events[1].Type, events[1].Error)
	}

	if s.Content() != "" {
		t.Errorf("content: expected empty, got %q", s.Content())
	}

	err := s.Err()
	if err == nil {
		t.Fatal("expected error from Err()")
	}
	streamErr, ok := err.(*StreamError)
	if !ok {
		t.Fatalf("expected *StreamError, got %T", err)
	}
	if streamErr.Message != expectedMsg {
		t.Errorf("expected %q, got %q", expectedMsg, streamErr.Message)
	}
}

func TestCrossSDK_ErrorOnly(t *testing.T) {
	s := streamFromFixture(fixtureErrorOnly)
	events := drainEvents(s)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "error" || events[0].Error != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %s %q", events[0].Type, events[0].Error)
	}

	if s.Content() != "" {
		t.Errorf("content: expected empty, got %q", s.Content())
	}

	err := s.Err()
	if err == nil {
		t.Fatal("expected error from Err()")
	}
	streamErr, ok := err.(*StreamError)
	if !ok {
		t.Fatalf("expected *StreamError, got %T", err)
	}
	if streamErr.Message != "unauthorized" {
		t.Errorf("expected 'unauthorized', got %q", streamErr.Message)
	}
}
