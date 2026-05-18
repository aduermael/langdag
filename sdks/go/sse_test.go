package langdag

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
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
	if node.Content != "Hello world!" {
		t.Errorf("expected streamed content fallback, got %q", node.Content)
	}
}

func TestStream_NodeMapsDoneResponseFields(t *testing.T) {
	input := `event: delta
data: {"content":"fallback content"}

event: done
data: {"node_id":"node-rich","content":"done content","tokens_in":10,"tokens_out":20,"tokens_cache_read":3,"tokens_cache_creation":4,"tokens_reasoning":5,"usage":{"input_tokens":10,"output_tokens":20},"metadata":{"normalized_usage":{"cache_creation_input_tokens":4}},"cost":{"status":"known","total":0.00042,"currency":"USD","source":"catalog"}}

`
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var doneEvent *SSEEvent
	for event := range stream.Events() {
		if event.Type == "done" {
			event := event
			doneEvent = &event
		}
	}
	if doneEvent == nil {
		t.Fatal("expected done event")
	}
	if doneEvent.Response == nil {
		t.Fatal("done event response is nil")
	}
	if doneEvent.Response.Content != "done content" {
		t.Errorf("done response content = %q, want %q", doneEvent.Response.Content, "done content")
	}

	node, err := stream.Node()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Content != "done content" {
		t.Errorf("Content = %q, want done response content", node.Content)
	}
	if node.TokensIn != 10 || node.TokensOut != 20 || node.TokensCacheRead != 3 ||
		node.TokensCacheCreation != 4 || node.TokensReasoning != 5 {
		t.Errorf("token fields were not mapped: %+v", node)
	}
	if node.Usage == nil || node.Usage.OutputTokens != 20 {
		t.Errorf("Usage = %+v, want output tokens 20", node.Usage)
	}
	if node.Metadata == nil || node.Metadata.NormalizedUsage == nil ||
		node.Metadata.NormalizedUsage.CacheCreationInputTokens != 4 {
		t.Errorf("Metadata = %+v, want cache creation tokens 4", node.Metadata)
	}
	if node.Cost == nil || node.Cost.Total != 0.00042 {
		t.Errorf("Cost = %+v, want total 0.00042", node.Cost)
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

func TestStream_NoDoneEvent(t *testing.T) {
	// Server sends start + deltas, then connection closes without done event
	input := `event: start
data: {}

event: delta
data: {"content":"Hello "}

event: delta
data: {"content":"world!"}

`
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	// Should have received start + 2 deltas
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Node() should return error (no done event)
	_, err := stream.Node()
	if err == nil {
		t.Fatal("expected error from Node() without done event")
	}
	streamErr, ok := err.(*StreamError)
	if !ok {
		t.Fatalf("expected *StreamError, got %T", err)
	}
	if streamErr.Message != "stream completed without node_id" {
		t.Errorf("unexpected error message: %q", streamErr.Message)
	}

	// Content() should still return accumulated text
	if stream.Content() != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", stream.Content())
	}
}

func TestStream_MalformedAmongValidDeltas(t *testing.T) {
	// One malformed delta among valid ones: valid content should still accumulate
	input := `event: start
data: {}

event: delta
data: {"content":"Hello "}

event: delta
data: {CORRUPT}

event: delta
data: {"content":"world!"}

event: done
data: {"node_id":"n-ok"}

`
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	var events []SSEEvent
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Malformed delta is emitted with empty Content (not skipped)
	if events[2].Type != "delta" || events[2].Content != "" {
		t.Errorf("malformed delta: expected empty content, got %q", events[2].Content)
	}

	// Content() accumulates only successfully parsed deltas
	if stream.Content() != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", stream.Content())
	}

	// No stream-level error: malformed JSON is silently ignored per-event
	if stream.Err() != nil {
		t.Errorf("expected no stream error, got %v", stream.Err())
	}

	// Node() should succeed (done event was received)
	node, err := stream.Node()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.ID != "n-ok" {
		t.Errorf("expected n-ok, got %s", node.ID)
	}
}

func TestStream_ErrMethod(t *testing.T) {
	// Verify Err() returns nil on success, non-nil on error
	t.Run("success", func(t *testing.T) {
		input := "event: done\ndata: {\"node_id\":\"n-1\"}\n\n"
		body := io.NopCloser(strings.NewReader(input))
		stream := newStream(body, nil)
		for range stream.Events() {
		}
		if stream.Err() != nil {
			t.Errorf("expected nil error, got %v", stream.Err())
		}
	})

	t.Run("error_event", func(t *testing.T) {
		input := "event: error\ndata: provider crashed\n\n"
		body := io.NopCloser(strings.NewReader(input))
		stream := newStream(body, nil)
		for range stream.Events() {
		}
		err := stream.Err()
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
	})

	t.Run("io_error", func(t *testing.T) {
		r := &errorReader{
			data: "event: start\ndata: {}\n\n",
			err:  errors.New("network failure"),
		}
		body := io.NopCloser(r)
		stream := newStream(body, nil)
		for range stream.Events() {
		}
		err := stream.Err()
		if err == nil {
			t.Fatal("expected error from Err()")
		}
		if err.Error() != "network failure" {
			t.Errorf("expected 'network failure', got %q", err.Error())
		}
	})
}

func TestStream_NoDoneEvent_ConnectionClose(t *testing.T) {
	// Simulates abrupt connection close after partial deltas (no trailing newline)
	input := "event: start\ndata: {}\n\nevent: delta\ndata: {\"content\":\"partial\"}"
	body := io.NopCloser(strings.NewReader(input))
	stream := newStream(body, nil)

	done := make(chan struct{})
	go func() {
		for range stream.Events() {
		}
		close(done)
	}()

	// Verify it doesn't hang — completes within timeout
	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("stream hung — Events() channel not closed after connection drop")
	}

	// Content should have the accumulated partial delta
	if stream.Content() != "partial" {
		t.Errorf("expected 'partial', got %q", stream.Content())
	}

	// Node() should return error
	_, err := stream.Node()
	if err == nil {
		t.Fatal("expected error from Node()")
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
