package langdag

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// SSEEventType represents the type of an SSE event.
type SSEEventType string

const (
	SSEEventStart SSEEventType = "start"
	SSEEventDelta SSEEventType = "delta"
	SSEEventDone  SSEEventType = "done"
	SSEEventError SSEEventType = "error"
)

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	Type SSEEventType
	Data string

	// Parsed fields (populated based on event type)
	DAGID   string // For start and done events
	NodeID  string // For done events
	Content string // For delta events
	Error   string // For error events
}

// SSEEventHandler is a callback function for handling SSE events.
type SSEEventHandler func(event SSEEvent) error

// parseSSEStream reads SSE events from a reader and calls the handler for each event.
func parseSSEStream(reader io.Reader, handler SSEEventHandler) error {
	scanner := bufio.NewScanner(reader)
	var eventType SSEEventType
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line signals end of event
		if line == "" {
			if eventType != "" && len(dataLines) > 0 {
				event := SSEEvent{
					Type: eventType,
					Data: strings.Join(dataLines, "\n"),
				}

				// Parse the data based on event type
				parseEventData(&event)

				if err := handler(event); err != nil {
					return err
				}
			}
			eventType = ""
			dataLines = nil
			continue
		}

		// Parse the field
		if strings.HasPrefix(line, "event:") {
			eventType = SSEEventType(strings.TrimSpace(strings.TrimPrefix(line, "event:")))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			// SSE spec says to remove leading space if present
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
			dataLines = append(dataLines, data)
		}
		// Ignore other fields (id, retry, comments)
	}

	// Handle any remaining event
	if eventType != "" && len(dataLines) > 0 {
		event := SSEEvent{
			Type: eventType,
			Data: strings.Join(dataLines, "\n"),
		}
		parseEventData(&event)
		if err := handler(event); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// parseEventData parses the event data and populates the appropriate fields.
func parseEventData(event *SSEEvent) {
	switch event.Type {
	case SSEEventStart:
		var data struct {
			DAGID string `json:"dag_id"`
		}
		if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
			event.DAGID = data.DAGID
		}

	case SSEEventDelta:
		var data struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
			event.Content = data.Content
		}

	case SSEEventDone:
		var data struct {
			DAGID  string `json:"dag_id"`
			NodeID string `json:"node_id"`
		}
		if err := json.Unmarshal([]byte(event.Data), &data); err == nil {
			event.DAGID = data.DAGID
			event.NodeID = data.NodeID
		}

	case SSEEventError:
		// Error data is plain text
		event.Error = event.Data
	}
}
