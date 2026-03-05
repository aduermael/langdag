// Package langgraph provides types and tools for importing LangGraph data into langdag.
package langgraph

import (
	"encoding/json"
	"time"
)

// ExportData is the top-level export from the Python LangGraph exporter.
type ExportData struct {
	Version    string         `json:"version"`
	SourceType string         `json:"source_type"`
	ExportedAt time.Time      `json:"exported_at"`
	Threads    []ExportThread `json:"threads"`
}

// ExportThread represents a single LangGraph thread (conversation).
type ExportThread struct {
	ThreadID  string          `json:"thread_id"`
	CreatedAt *time.Time      `json:"created_at,omitempty"`
	Messages  []ExportMessage `json:"messages"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// ExportMessage represents a single message in a thread.
type ExportMessage struct {
	ID         string           `json:"id"`
	Role       string           `json:"role"` // "user", "assistant", "tool", "system"
	Content    string           `json:"content"`
	CreatedAt  *time.Time       `json:"created_at,omitempty"`
	Model      string           `json:"model,omitempty"`
	TokensIn   int              `json:"tokens_in,omitempty"`
	TokensOut  int              `json:"tokens_out,omitempty"`
	ToolCalls  []ExportToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolName   string           `json:"tool_name,omitempty"`
}

// ExportToolCall represents a tool call within an assistant message.
type ExportToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}
