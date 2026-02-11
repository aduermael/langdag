package langdag

import (
	"io"
	"strings"
	"testing"
)

func TestStream_StartDeltaDone(t *testing.T) {
	input := `event: start
data: {}

event: delta
data: {"content":"Hello "}

event: delta
data: {"content":"world!"}

event: done
data: {"node_id":"node-456"}

`
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	if events[0].Type != "start" {
		t.Errorf("expected start, got %s", events[0].Type)
	}

	if events[1].Type != "delta" || events[1].Content != "Hello " {
		t.Errorf("expected delta 'Hello ', got %s %q", events[1].Type, events[1].Content)
	}

	if events[2].Type != "delta" || events[2].Content != "world!" {
		t.Errorf("expected delta 'world!', got %s %q", events[2].Type, events[2].Content)
	}

	if events[3].Type != "done" || events[3].NodeID != "node-456" {
		t.Errorf("expected done with node-456, got %s %s", events[3].Type, events[3].NodeID)
	}

	node, err := stream.Node()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.ID != "node-456" {
		t.Errorf("expected node-456, got %s", node.ID)
	}
}

func TestStream_ErrorEvent(t *testing.T) {
	input := `event: error
data: something went wrong

`
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "error" {
		t.Errorf("expected error, got %s", events[0].Type)
	}
	if events[0].Error != "something went wrong" {
		t.Errorf("expected error message, got %q", events[0].Error)
	}

	_, err := stream.Node()
	if err == nil {
		t.Error("expected error from Node() after error event")
	}
}

func TestStream_EmptyStream(t *testing.T) {
	body := io.NopCloser(strings.NewReader(""))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}

	_, err := stream.Node()
	if err == nil {
		t.Error("expected error from Node() on empty stream")
	}
}

func TestStream_NoTrailingNewline(t *testing.T) {
	input := `event: done
data: {"node_id":"n-1"}`

	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].NodeID != "n-1" {
		t.Errorf("expected n-1, got %s", events[0].NodeID)
	}
}

func TestStream_CollectContent(t *testing.T) {
	input := `event: start
data: {}

event: delta
data: {"content":"Hello "}

event: delta
data: {"content":"world"}

event: delta
data: {"content":"!"}

event: done
data: {"node_id":"n-1"}

`
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var content strings.Builder
	for event := range stream.Events() {
		if event.Type == "delta" {
			content.WriteString(event.Content)
		}
	}

	if content.String() != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", content.String())
	}
}
