// Package langdag provides a Go client for the LangDAG REST API.
package langdag

import (
	"context"
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
	Sequence     int       `json:"sequence"`
	Type         NodeType  `json:"node_type"`
	Content      string    `json:"content"`
	Model        string    `json:"model,omitempty"`
	TokensIn     int       `json:"tokens_in,omitempty"`
	TokensOut    int       `json:"tokens_out,omitempty"`
	LatencyMs    int       `json:"latency_ms,omitempty"`
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

// PromptOption configures a prompt request.
type PromptOption func(*promptOptions)

type promptOptions struct {
	model        string
	systemPrompt string
}

// WithSystem sets the system prompt (only for new trees via client.Prompt).
func WithSystem(prompt string) PromptOption {
	return func(o *promptOptions) {
		o.systemPrompt = prompt
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
	Message      string `json:"message"`
	Model        string `json:"model,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	Stream       bool   `json:"stream,omitempty"`
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

// Workflow represents a workflow template.
type Workflow struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     int       `json:"version"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkflowDefaults represents default settings for a workflow.
type WorkflowDefaults struct {
	Provider    string  `json:"provider,omitempty"`
	Model       string  `json:"model,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// ToolDefinition represents a tool definition in a workflow.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// WorkflowNodeType represents the type of a workflow node.
type WorkflowNodeType string

const (
	WorkflowNodeTypeLLM    WorkflowNodeType = "llm"
	WorkflowNodeTypeTool   WorkflowNodeType = "tool"
	WorkflowNodeTypeBranch WorkflowNodeType = "branch"
	WorkflowNodeTypeMerge  WorkflowNodeType = "merge"
	WorkflowNodeTypeInput  WorkflowNodeType = "input"
	WorkflowNodeTypeOutput WorkflowNodeType = "output"
)

// WorkflowNode represents a node in a workflow definition.
type WorkflowNode struct {
	ID        string                 `json:"id"`
	Type      WorkflowNodeType       `json:"type"`
	Content   map[string]interface{} `json:"content,omitempty"`
	Model     string                 `json:"model,omitempty"`
	System    string                 `json:"system,omitempty"`
	Prompt    string                 `json:"prompt,omitempty"`
	Tools     []string               `json:"tools,omitempty"`
	Handler   string                 `json:"handler,omitempty"`
	Condition string                 `json:"condition,omitempty"`
}

// WorkflowEdge represents an edge connecting nodes in a workflow.
type WorkflowEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
	Transform string `json:"transform,omitempty"`
}

// CreateWorkflowRequest represents a request to create a new workflow.
type CreateWorkflowRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Defaults    *WorkflowDefaults `json:"defaults,omitempty"`
	Tools       []ToolDefinition  `json:"tools,omitempty"`
	Nodes       []WorkflowNode    `json:"nodes"`
	Edges       []WorkflowEdge    `json:"edges,omitempty"`
}

// RunWorkflowRequest represents a request to run a workflow.
type RunWorkflowRequest struct {
	Input  map[string]interface{} `json:"input,omitempty"`
	Stream bool                   `json:"stream,omitempty"`
}

// RunWorkflowResponse represents the response from running a workflow.
type RunWorkflowResponse struct {
	NodeID string                 `json:"node_id"`
	Status string                 `json:"status"`
	Output map[string]interface{} `json:"output,omitempty"`
}
