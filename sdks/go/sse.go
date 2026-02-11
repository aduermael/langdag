package langdag

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	Type    string
	Content string // For delta events
	NodeID  string // For done events
	Error   string // For error events
}

// Stream wraps an SSE response and provides a channel-based API.
type Stream struct {
	events chan SSEEvent
	body   io.ReadCloser
	client *Client
	nodeID string
	err    error
}

// newStream creates a new Stream from an HTTP response body.
func newStream(body io.ReadCloser, client *Client) *Stream {
	s := &Stream{
		events: make(chan SSEEvent, 64),
		body:   body,
		client: client,
	}
	go s.read()
	return s
}

// Events returns a channel that yields SSE events.
func (s *Stream) Events() <-chan SSEEvent {
	return s.events
}

// Node waits for the stream to complete and returns the resulting node.
// Must be called after draining Events().
func (s *Stream) Node() (*Node, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.nodeID == "" {
		return nil, &StreamError{Message: "stream completed without node_id"}
	}
	return &Node{
		ID:     s.nodeID,
		Type:   NodeTypeAssistant,
		client: s.client,
	}, nil
}

// read parses SSE events from the body and sends them on the channel.
func (s *Stream) read() {
	defer close(s.events)
	defer s.body.Close()

	scanner := bufio.NewScanner(s.body)
	var eventType string
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if eventType != "" && len(dataLines) > 0 {
				event := s.parseEvent(eventType, strings.Join(dataLines, "\n"))
				if event.Type == "done" {
					s.nodeID = event.NodeID
				}
				if event.Type == "error" {
					s.err = &StreamError{Message: event.Error}
				}
				s.events <- event
			}
			eventType = ""
			dataLines = nil
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
			dataLines = append(dataLines, data)
		}
	}

	// Handle any remaining event without trailing newline
	if eventType != "" && len(dataLines) > 0 {
		event := s.parseEvent(eventType, strings.Join(dataLines, "\n"))
		if event.Type == "done" {
			s.nodeID = event.NodeID
		}
		if event.Type == "error" {
			s.err = &StreamError{Message: event.Error}
		}
		s.events <- event
	}

	if err := scanner.Err(); err != nil {
		s.err = err
	}
}

// parseEvent converts raw SSE data into a typed SSEEvent.
func (s *Stream) parseEvent(eventType, data string) SSEEvent {
	event := SSEEvent{Type: eventType}

	switch eventType {
	case "start":
		// Start event has no meaningful data
	case "delta":
		var d struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(data), &d); err == nil {
			event.Content = d.Content
		}
	case "done":
		var d struct {
			NodeID string `json:"node_id"`
		}
		if err := json.Unmarshal([]byte(data), &d); err == nil {
			event.NodeID = d.NodeID
		}
	case "error":
		event.Error = data
	}

	return event
}
