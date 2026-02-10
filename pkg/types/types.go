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

// NodeType represents the type of a node.
type NodeType string

const (
	// Conversation tree node types
	NodeTypeUser       NodeType = "user"
	NodeTypeAssistant  NodeType = "assistant"
	NodeTypeSystem     NodeType = "system"
	NodeTypeToolCall   NodeType = "tool_call"
	NodeTypeToolResult NodeType = "tool_result"

	// Workflow template node types
	NodeTypeLLM    NodeType = "llm"
	NodeTypeTool   NodeType = "tool"
	NodeTypeBranch NodeType = "branch"
	NodeTypeMerge  NodeType = "merge"
	NodeTypeInput  NodeType = "input"
	NodeTypeOutput NodeType = "output"
)

// Node represents a node in the conversation/workflow tree.
// Root nodes (ParentID == "") define the start of a tree and carry
// metadata like Title and SystemPrompt.
type Node struct {
	ID       string   `json:"id"`
	ParentID string   `json:"parent_id,omitempty"`
	Sequence int      `json:"sequence"`
	NodeType NodeType `json:"node_type"`
	Content  string   `json:"content"`

	// LLM execution metadata (on assistant nodes)
	Model     string `json:"model,omitempty"`
	TokensIn  int    `json:"tokens_in,omitempty"`
	TokensOut int    `json:"tokens_out,omitempty"`
	LatencyMs int    `json:"latency_ms,omitempty"`
	Status    string `json:"status,omitempty"`

	// Root node metadata (empty on non-root nodes)
	Title        string `json:"title,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// Tree represents a tree of nodes rooted at a specific node.
type Tree struct {
	Root  *Node  `json:"root"`
	Nodes []Node `json:"nodes"`
}

// WorkflowNode represents a node in a workflow template.
type WorkflowNode struct {
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
	Nodes       []WorkflowNode   `json:"nodes"`
	Edges       []Edge           `json:"edges"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
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
	StreamEventNodeSaved   StreamEventType = "node_saved"
)

// StreamEvent represents an event during streaming completion.
type StreamEvent struct {
	Type         StreamEventType     `json:"type"`
	Content      string              `json:"content,omitempty"`       // For delta events
	ContentBlock *ContentBlock       `json:"content_block,omitempty"` // For content_done events
	Response     *CompletionResponse `json:"response,omitempty"`      // For done events
	Error        error               `json:"-"`                       // For error events
	NodeID       string              `json:"node_id,omitempty"`       // For node_saved events
}

// ModelInfo represents information about a model.
type ModelInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
}
