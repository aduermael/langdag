// Cross-SDK graceful degradation tests — verify the Go SDK makes accumulated
// content available when streams terminate abnormally (no done event).
// Identical scenarios tested in Python and TypeScript SDKs.

package langdag

import (
	"io"
	"strings"
	"testing"
	"time"
)

// Fixture: stream ends without done event (server closes connection)
const fixtureDegradationNoDone = "event: start\ndata: {}\n\n" +
	"event: delta\ndata: {\"content\":\"Hello \"}\n\n" +
	"event: delta\ndata: {\"content\":\"world!\"}\n\n"

// Fixture: stream ends with error event (no done)
const fixtureDegradationErrorTermination = "event: start\ndata: {}\n\n" +
	"event: delta\ndata: {\"content\":\"before \"}\n\n" +
	"event: delta\ndata: {\"content\":\"error\"}\n\n" +
	"event: error\ndata: connection reset by peer\n\n"

// Fixture: stream has only start, then closes (empty response)
const fixtureDegradationEmptyResponse = "event: start\ndata: {}\n\n"

func TestGracefulDeg_NoDoneEvent_ContentAvailable(t *testing.T) {
	s := newStream(io.NopCloser(strings.NewReader(fixtureDegradationNoDone)), nil)
	for range s.Events() {
	}

	// Content must be available even without done event
	if s.Content() != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", s.Content())
	}

	// Node() must return an error (not hang, not panic)
	_, err := s.Node()
	if err == nil {
		t.Fatal("expected error from Node() without done event")
	}

	// Error should indicate missing node_id, not be a random crash
	streamErr, ok := err.(*StreamError)
	if !ok {
		t.Fatalf("expected *StreamError, got %T: %v", err, err)
	}
	if streamErr.Message != "stream completed without node_id" {
		t.Errorf("unexpected message: %q", streamErr.Message)
	}
}

func TestGracefulDeg_ErrorTermination_ContentPreserved(t *testing.T) {
	s := newStream(io.NopCloser(strings.NewReader(fixtureDegradationErrorTermination)), nil)
	for range s.Events() {
	}

	// Content accumulated before the error must be preserved
	if s.Content() != "before error" {
		t.Errorf("expected 'before error', got %q", s.Content())
	}

	// Err() must surface the error message
	err := s.Err()
	if err == nil {
		t.Fatal("expected error from Err()")
	}
	streamErr, ok := err.(*StreamError)
	if !ok {
		t.Fatalf("expected *StreamError, got %T", err)
	}
	if streamErr.Message != "connection reset by peer" {
		t.Errorf("unexpected message: %q", streamErr.Message)
	}
}

func TestGracefulDeg_EmptyResponse_NoHang(t *testing.T) {
	s := newStream(io.NopCloser(strings.NewReader(fixtureDegradationEmptyResponse)), nil)

	done := make(chan struct{})
	go func() {
		for range s.Events() {
		}
		close(done)
	}()

	select {
	case <-done:
		// good — completed without hanging
	case <-time.After(2 * time.Second):
		t.Fatal("stream hung on empty response")
	}

	// Content should be empty
	if s.Content() != "" {
		t.Errorf("expected empty content, got %q", s.Content())
	}
}

func TestGracefulDeg_IOError_ContentPreserved(t *testing.T) {
	// Simulate connection drop mid-stream: some data, then I/O error
	r := &errorReader{
		data: "event: start\ndata: {}\n\nevent: delta\ndata: {\"content\":\"before drop\"}\n\n",
		err:  io.ErrUnexpectedEOF,
	}
	s := newStream(io.NopCloser(r), nil)

	done := make(chan struct{})
	go func() {
		for range s.Events() {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream hung on I/O error")
	}

	if s.Content() != "before drop" {
		t.Errorf("expected 'before drop', got %q", s.Content())
	}

	// Err() should return the I/O error
	if s.Err() == nil {
		t.Fatal("expected error from Err()")
	}
}
