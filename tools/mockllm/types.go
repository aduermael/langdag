package main

import "encoding/json"

// MessagesRequest represents an Anthropic /v1/messages request.
type MessagesRequest struct {
	Model         string          `json:"model"`
	Messages      []Message       `json:"messages"`
	MaxTokens     int             `json:"max_tokens"`
	System        json.RawMessage `json:"system,omitempty"` // string or []TextBlock
	Stream        bool            `json:"stream,omitempty"`
	Temperature   float64         `json:"temperature,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []Tool          `json:"tools,omitempty"`
}

// Message represents a message in the request.
type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []ContentBlock
}

// ContentBlock represents a content block in a message or response.
type ContentBlock struct {
	Type string `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result block
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Tool represents a tool definition in the request.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// MessagesResponse represents an Anthropic /v1/messages response.
type MessagesResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Model      string         `json:"model"`
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

// Usage represents token usage.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
