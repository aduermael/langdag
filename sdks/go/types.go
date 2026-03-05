// Package langdag provides a Go client for the LangDAG REST API.
package langdag

import (
	"context"
	"encoding/json"
	"time"
)

// NodeType represents the type of a node.
type NodeType string

const (
	NodeTypeUser       NodeType = "user"
	NodeTypeAssistant  NodeType = "assistant"
	NodeTypeToolCall   NodeType = "tool_call"
	NodeTypeToolResult NodeType = "tool_result"
)

// Node represents a node in a conversation tree.
// Root nodes (ParentID == "") carry metadata like Title and SystemPrompt.
type Node struct {
	ID           string    `json:"id"`
	ParentID     string    `json:"parent_id,omitempty"`
	RootID       string    `json:"root_id,omitempty"`
	Sequence     int       `json:"sequence"`
	Type         NodeType  `json:"node_type"`
	Content      string    `json:"content"`
	Model        string    `json:"model,omitempty"`
	TokensIn     int       `json:"tokens_in,omitempty"`
	TokensOut           int       `json:"tokens_out,omitempty"`
	TokensCacheRead     int       `json:"tokens_cache_read,omitempty"`
	TokensCacheCreation int       `json:"tokens_cache_creation,omitempty"`
	TokensReasoning     int       `json:"tokens_reasoning,omitempty"`
	LatencyMs           int       `json:"latency_ms,omitempty"`
	Status       string    `json:"status,omitempty"`
	Title        string    `json:"title,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	CreatedAt    time.Time `json:"created_at"`

	client *Client // unexported — enables Prompt()
}

// Prompt continues the conversation from this node.
func (n *Node) Prompt(ctx context.Context, message string, opts ...PromptOption) (*Node, error) {
	o := &promptOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return n.client.promptFrom(ctx, n.ID, message, o)
}

// PromptStream continues the conversation from this node with streaming.
func (n *Node) PromptStream(ctx context.Context, message string, opts ...PromptOption) (*Stream, error) {
	o := &promptOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return n.client.promptStreamFrom(ctx, n.ID, message, o)
}

// Tree represents a tree of nodes.
type Tree struct {
	Nodes []Node `json:"nodes"`
}

// ToolDefinition describes a tool that the model can use.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// PromptOption configures a prompt request.
type PromptOption func(*promptOptions)

type promptOptions struct {
	model        string
	systemPrompt string
	tools        []ToolDefinition
}

// WithSystem sets the system prompt (only for new trees via client.Prompt).
func WithSystem(prompt string) PromptOption {
	return func(o *promptOptions) {
		o.systemPrompt = prompt
	}
}

// WithTools sets the tools available for the prompt.
func WithTools(tools []ToolDefinition) PromptOption {
	return func(o *promptOptions) {
		o.tools = tools
	}
}

// WithModel sets the model for the prompt.
func WithModel(model string) PromptOption {
	return func(o *promptOptions) {
		o.model = model
	}
}

// promptRequest is the JSON body sent to /prompt and /nodes/{id}/prompt.
type promptRequest struct {
	Message      string           `json:"message"`
	Model        string           `json:"model,omitempty"`
	SystemPrompt string           `json:"system_prompt,omitempty"`
	Stream       bool             `json:"stream,omitempty"`
	Tools        []ToolDefinition `json:"tools,omitempty"`
}

// promptResponse is the JSON body returned from /prompt and /nodes/{id}/prompt.
type promptResponse struct {
	NodeID    string `json:"node_id"`
	Content   string `json:"content"`
	TokensIn  int    `json:"tokens_in,omitempty"`
	TokensOut int    `json:"tokens_out,omitempty"`
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status string `json:"status"`
}

// DeleteResponse represents a delete response.
type DeleteResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

