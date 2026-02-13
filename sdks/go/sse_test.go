package langdag

import (
	"errors"
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

func TestStream_MalformedDeltaJSON(t *testing.T) {
	input := "event: delta\ndata: {not valid json}\n\nevent: done\ndata: {\"node_id\":\"n-1\"}\n\n"
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Malformed delta should still be emitted but with empty Content
	if events[0].Type != "delta" {
		t.Errorf("expected delta, got %s", events[0].Type)
	}
	if events[0].Content != "" {
		t.Errorf("expected empty content for malformed delta, got %q", events[0].Content)
	}
}

func TestStream_MalformedDoneJSON(t *testing.T) {
	input := "event: done\ndata: not-json\n\n"
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].NodeID != "" {
		t.Errorf("expected empty NodeID for malformed done, got %q", events[0].NodeID)
	}

	// Node() should return error because nodeID was never set
	_, err := stream.Node()
	if err == nil {
		t.Error("expected error from Node() with malformed done event")
	}
}

func TestStream_EmptyDataField(t *testing.T) {
	input := "event: delta\ndata: \n\nevent: done\ndata: {\"node_id\":\"n-2\"}\n\n"
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Empty data field on delta: JSON unmarshal will fail, Content stays ""
	if events[0].Content != "" {
		t.Errorf("expected empty content, got %q", events[0].Content)
	}
}

func TestStream_ScannerError(t *testing.T) {
	// errorReader simulates an I/O error mid-stream
	r := &errorReader{
		data: "event: start\ndata: {}\n\n",
		err:  errors.New("connection reset"),
	}
	body := io.NopCloser(r)
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	// Should have emitted the start event before the error
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "start" {
		t.Errorf("expected start, got %s", events[0].Type)
	}

	// stream.err should be set from the scanner error
	_, err := stream.Node()
	if err == nil {
		t.Error("expected error from Node() after scanner error")
	}
}

func TestStream_MultipleErrorEvents(t *testing.T) {
	input := "event: error\ndata: first error\n\nevent: error\ndata: second error\n\n"
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// The last error should be the one stored in s.err
	_, err := stream.Node()
	if err == nil {
		t.Fatal("expected error from Node()")
	}
	streamErr, ok := err.(*StreamError)
	if !ok {
		t.Fatalf("expected *StreamError, got %T", err)
	}
	if streamErr.Message != "second error" {
		t.Errorf("expected 'second error', got %q", streamErr.Message)
	}
}

func TestStream_MultilineDataField(t *testing.T) {
	// SSE spec: multiple data: lines get joined with newlines
	input := "event: delta\ndata: {\"content\":\n data: \"hello\"}\n\n"
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	// The two data lines get joined with "\n", result is: {"content":\n"hello"}
	// This is invalid JSON so Content will be empty (no crash)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "delta" {
		t.Errorf("expected delta, got %s", events[0].Type)
	}
}

// errorReader returns data first, then an error
type errorReader struct {
	data string
	err  error
	pos  int
}

func (r *errorReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, r.err
	}
	return n, nil
}
