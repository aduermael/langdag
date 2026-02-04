package langdag

import (
	"fmt"
)

// APIError represents an error returned by the LangDAG API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("langdag: API error (status %d): %s", e.StatusCode, e.Message)
}

// IsNotFound returns true if the error is a 404 Not Found error.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
}

// IsUnauthorized returns true if the error is a 401 Unauthorized error.
func (e *APIError) IsUnauthorized() bool {
	return e.StatusCode == 401
}

// IsBadRequest returns true if the error is a 400 Bad Request error.
func (e *APIError) IsBadRequest() bool {
	return e.StatusCode == 400
}

// NotFoundError is returned when a resource is not found.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("langdag: %s not found: %s", e.Resource, e.ID)
}

// StreamError represents an error that occurred during SSE streaming.
type StreamError struct {
	Message string
}

func (e *StreamError) Error() string {
	return fmt.Sprintf("langdag: stream error: %s", e.Message)
}

// ConnectionError represents a connection error.
type ConnectionError struct {
	Err error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("langdag: connection error: %v", e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}
