package langdag

import (
	"strings"
	"testing"
)

func TestParseSSEStream_StartDeltaDone(t *testing.T) {
	input := `event: start
data: {"dag_id":"dag-123"}

event: delta
data: {"content":"Hello "}

event: delta
data: {"content":"world!"}

event: done
data: {"dag_id":"dag-123","node_id":"node-456"}

`
	var events []SSEEvent
	err := parseSSEStream(strings.NewReader(input), func(e SSEEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// start event
	if events[0].Type != SSEEventStart {
		t.Errorf("expected start, got %s", events[0].Type)
	}
	if events[0].DAGID != "dag-123" {
		t.Errorf("expected dag-123, got %s", events[0].DAGID)
	}

	// delta events
	if events[1].Type != SSEEventDelta {
		t.Errorf("expected delta, got %s", events[1].Type)
	}
	if events[1].Content != "Hello " {
		t.Errorf("expected 'Hello ', got %q", events[1].Content)
	}
	if events[2].Content != "world!" {
		t.Errorf("expected 'world!', got %q", events[2].Content)
	}

	// done event
	if events[3].Type != SSEEventDone {
		t.Errorf("expected done, got %s", events[3].Type)
	}
	if events[3].DAGID != "dag-123" {
		t.Errorf("expected dag-123, got %s", events[3].DAGID)
	}
	if events[3].NodeID != "node-456" {
		t.Errorf("expected node-456, got %s", events[3].NodeID)
	}
}

func TestParseSSEStream_ErrorEvent(t *testing.T) {
	input := `event: error
data: something went wrong

`
	var events []SSEEvent
	err := parseSSEStream(strings.NewReader(input), func(e SSEEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != SSEEventError {
		t.Errorf("expected error, got %s", events[0].Type)
	}
	if events[0].Error != "something went wrong" {
		t.Errorf("expected error message, got %q", events[0].Error)
	}
}

func TestParseSSEStream_MultiLineData(t *testing.T) {
	input := `event: delta
data: {"content":
data: "hello"}

`
	var events []SSEEvent
	err := parseSSEStream(strings.NewReader(input), func(e SSEEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	// Multi-line data should be joined with newlines
	if events[0].Data != "{\"content\":\n\"hello\"}" {
		t.Errorf("expected multi-line data, got %q", events[0].Data)
	}
}

func TestParseSSEStream_EmptyStream(t *testing.T) {
	var events []SSEEvent
	err := parseSSEStream(strings.NewReader(""), func(e SSEEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestParseSSEStream_NoTrailingNewline(t *testing.T) {
	// Events without trailing newline should still be processed
	input := `event: start
data: {"dag_id":"dag-1"}`

	var events []SSEEvent
	err := parseSSEStream(strings.NewReader(input), func(e SSEEvent) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].DAGID != "dag-1" {
		t.Errorf("expected dag-1, got %s", events[0].DAGID)
	}
}

func TestParseSSEStream_HandlerError(t *testing.T) {
	input := `event: start
data: {"dag_id":"dag-1"}

event: delta
data: {"content":"hello"}

`
	callCount := 0
	err := parseSSEStream(strings.NewReader(input), func(e SSEEvent) error {
		callCount++
		if callCount == 1 {
			return &StreamError{Message: "stop"}
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected error from handler, got nil")
	}
	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}
}
