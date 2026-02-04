// Package types defines shared types used across the langdag codebase.
package types

import (
	"encoding/json"
	"time"
)

// Message represents a single message in a conversation.
type Message struct {
	Role    string          `json:"role"`    // "user", "assistant", "tool_result"
	Content json.RawMessage `json:"content"` // string or []ContentBlock
}

// ContentBlock represents a content block in a message.
type ContentBlock struct {
	Type string `json:"type"` // "text", "image", "tool_use", "tool_result"

	// For text blocks
	Text string `json:"text,omitempty"`

	// For tool_use blocks
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// For tool_result blocks
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// NodeType represents the type of a node in a DAG.
type NodeType string

const (
	NodeTypeLLM        NodeType = "llm"
	NodeTypeTool       NodeType = "tool"
	NodeTypeBranch     NodeType = "branch"
	NodeTypeMerge      NodeType = "merge"
	NodeTypeInput      NodeType = "input"
	NodeTypeOutput     NodeType = "output"
	NodeTypeUser       NodeType = "user"
	NodeTypeAssistant  NodeType = "assistant"
	NodeTypeToolCall   NodeType = "tool_call"
	NodeTypeToolResult NodeType = "tool_result"
)

// Node represents a node in a workflow template.
type Node struct {
	ID        string          `json:"id"`
	Type      NodeType        `json:"type"`
	Content   json.RawMessage `json:"content,omitempty"`
	Model     string          `json:"model,omitempty"`
	System    string          `json:"system,omitempty"`
	Prompt    string          `json:"prompt,omitempty"`
	Tools     []string        `json:"tools,omitempty"`
	Handler   string          `json:"handler,omitempty"`
	Condition string          `json:"condition,omitempty"`
}

// Edge represents an edge connecting two nodes in a workflow.
type Edge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
	Transform string `json:"transform,omitempty"`
}

// WorkflowDefaults represents default settings for a workflow.
type WorkflowDefaults struct {
	Provider    string  `json:"provider,omitempty"`
	Model       string  `json:"model,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// ToolDefinition represents a tool that can be used in a workflow.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Workflow represents a workflow template (pre-defined DAG structure).
type Workflow struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Version     int              `json:"version"`
	Description string           `json:"description,omitempty"`
	Defaults    WorkflowDefaults `json:"defaults,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Nodes       []Node           `json:"nodes"`
	Edges       []Edge           `json:"edges"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// DAGStatus represents the status of a DAG instance.
type DAGStatus string

const (
	DAGStatusPending   DAGStatus = "pending"
	DAGStatusRunning   DAGStatus = "running"
	DAGStatusCompleted DAGStatus = "completed"
	DAGStatusFailed    DAGStatus = "failed"
	DAGStatusCancelled DAGStatus = "cancelled"
)

// DAG represents a DAG instance (unified from conversations and runs).
// A DAG can be created from a workflow template or started as a fresh chat.
type DAG struct {
	ID            string           `json:"id"`
	Title         string           `json:"title,omitempty"`
	WorkflowID    string           `json:"workflow_id,omitempty"` // NULL if started from chat
	Model         string           `json:"model,omitempty"`
	SystemPrompt  string           `json:"system_prompt,omitempty"`
	Tools         []ToolDefinition `json:"tools,omitempty"`
	Status        DAGStatus        `json:"status"`
	Input         json.RawMessage  `json:"input,omitempty"`
	Output        json.RawMessage  `json:"output,omitempty"`
	ForkedFromDAG string           `json:"forked_from_dag,omitempty"`
	ForkedFromNode string          `json:"forked_from_node,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// DAGNode represents a node in a DAG instance (unified from conversation_nodes and node_runs).
type DAGNode struct {
	ID        string          `json:"id"`
	DAGID     string          `json:"dag_id"`
	ParentID  string          `json:"parent_id,omitempty"`
	Sequence  int             `json:"sequence"`
	NodeType  NodeType        `json:"node_type"`
	Content   json.RawMessage `json:"content"`
	Model     string          `json:"model,omitempty"`
	TokensIn  int             `json:"tokens_in,omitempty"`
	TokensOut int             `json:"tokens_out,omitempty"`
	LatencyMs int             `json:"latency_ms,omitempty"`
	Status    DAGStatus       `json:"status,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// CompletionRequest represents a request to an LLM provider.
type CompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	System      string           `json:"system,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	StopSeqs    []string         `json:"stop_sequences,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
}

// CompletionResponse represents a response from an LLM provider.
type CompletionResponse struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

// Usage represents token usage information.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEventType represents the type of a streaming event.
type StreamEventType string

const (
	StreamEventStart       StreamEventType = "start"
	StreamEventDelta       StreamEventType = "delta"
	StreamEventContentDone StreamEventType = "content_done"
	StreamEventDone        StreamEventType = "done"
	StreamEventError       StreamEventType = "error"
)

// StreamEvent represents an event during streaming completion.
type StreamEvent struct {
	Type         StreamEventType     `json:"type"`
	Content      string              `json:"content,omitempty"`       // For delta events
	ContentBlock *ContentBlock       `json:"content_block,omitempty"` // For content_done events
	Response     *CompletionResponse `json:"response,omitempty"`      // For done events
	Error        error               `json:"-"`                       // For error events
}

// ModelInfo represents information about a model.
type ModelInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
}
