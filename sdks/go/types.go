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
	ID                  string                 `json:"id"`
	ParentID            string                 `json:"parent_id,omitempty"`
	RootID              string                 `json:"root_id,omitempty"`
	Sequence            int                    `json:"sequence"`
	Type                NodeType               `json:"node_type"`
	Content             string                 `json:"content"`
	Provider            string                 `json:"provider,omitempty"`
	Model               string                 `json:"model,omitempty"`
	TokensIn            int                    `json:"tokens_in,omitempty"`
	TokensOut           int                    `json:"tokens_out,omitempty"`
	TokensCacheRead     int                    `json:"tokens_cache_read,omitempty"`
	TokensCacheCreation int                    `json:"tokens_cache_creation,omitempty"`
	TokensReasoning     int                    `json:"tokens_reasoning,omitempty"`
	LatencyMs           int                    `json:"latency_ms,omitempty"`
	StopReason          string                 `json:"stop_reason,omitempty"`
	Status              string                 `json:"status,omitempty"`
	Title               string                 `json:"title,omitempty"`
	SystemPrompt        string                 `json:"system_prompt,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	Usage               *NormalizedUsage       `json:"usage,omitempty"`
	Metadata            *AssistantNodeMetadata `json:"metadata,omitempty"`
	Cost                *CostResult            `json:"cost,omitempty"`

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

// PromptResponse is the JSON body returned from /prompt and /nodes/{id}/prompt.
type PromptResponse struct {
	NodeID              string                 `json:"node_id"`
	Content             string                 `json:"content"`
	TokensIn            int                    `json:"tokens_in,omitempty"`
	TokensOut           int                    `json:"tokens_out,omitempty"`
	TokensCacheRead     int                    `json:"tokens_cache_read,omitempty"`
	TokensCacheCreation int                    `json:"tokens_cache_creation,omitempty"`
	TokensReasoning     int                    `json:"tokens_reasoning,omitempty"`
	Usage               *NormalizedUsage       `json:"usage,omitempty"`
	Metadata            *AssistantNodeMetadata `json:"metadata,omitempty"`
	Cost                *CostResult            `json:"cost,omitempty"`
}

func nodeFromPromptResponse(resp *PromptResponse, client *Client, fallbackContent string) *Node {
	node := &Node{
		Type:    NodeTypeAssistant,
		Content: fallbackContent,
		client:  client,
	}
	if resp == nil {
		return node
	}

	node.ID = resp.NodeID
	if resp.Content != "" {
		node.Content = resp.Content
	}
	node.TokensIn = resp.TokensIn
	node.TokensOut = resp.TokensOut
	node.TokensCacheRead = resp.TokensCacheRead
	node.TokensCacheCreation = resp.TokensCacheCreation
	node.TokensReasoning = resp.TokensReasoning
	node.Usage = resp.Usage
	node.Metadata = resp.Metadata
	node.Cost = resp.Cost
	return node
}

type NormalizedUsage struct {
	InputTokens              int              `json:"input_tokens,omitempty"`
	OutputTokens             int              `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int              `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int              `json:"cache_creation_input_tokens,omitempty"`
	CacheWriteInputTokens    int              `json:"cache_write_input_tokens,omitempty"`
	ReasoningTokens          int              `json:"reasoning_tokens,omitempty"`
	ToolUsePromptTokens      int              `json:"tool_use_prompt_tokens,omitempty"`
	AudioInputTokens         int              `json:"audio_input_tokens,omitempty"`
	AudioOutputTokens        int              `json:"audio_output_tokens,omitempty"`
	ImageInputTokens         int              `json:"image_input_tokens,omitempty"`
	ImageOutputTokens        int              `json:"image_output_tokens,omitempty"`
	AcceptedPredictionTokens int              `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int              `json:"rejected_prediction_tokens,omitempty"`
	ServiceTier              string           `json:"service_tier,omitempty"`
	Dimensions               map[string]int64 `json:"dimensions,omitempty"`
}

type ModelResolutionMetadata struct {
	CanonicalModelID string `json:"canonical_model_id"`
	OfferingID       string `json:"offering_id"`
	DeploymentID     string `json:"deployment_id"`
	ProviderID       string `json:"provider_id"`
	APIProtocolID    string `json:"api_protocol_id"`
	NativeModelID    string `json:"native_model_id"`
}

type PricingSnapshot struct {
	Status            string             `json:"status"`
	Currency          string             `json:"currency,omitempty"`
	EffectiveAt       time.Time          `json:"effective_at,omitempty"`
	Source            string             `json:"source,omitempty"`
	RatesPer1M        map[string]float64 `json:"rates_per_1m,omitempty"`
	MissingDimensions []string           `json:"missing_dimensions,omitempty"`
}

type ProviderCost struct {
	Total    float64         `json:"total"`
	Currency string          `json:"currency"`
	Source   string          `json:"source"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

type CostDimension struct {
	Name      string  `json:"name"`
	Quantity  int64   `json:"quantity"`
	RatePer1M float64 `json:"rate_per_1m"`
	Cost      float64 `json:"cost"`
}

type CostResult struct {
	Status            string          `json:"status"`
	Total             float64         `json:"total,omitempty"`
	Currency          string          `json:"currency,omitempty"`
	Source            string          `json:"source,omitempty"`
	MissingDimensions []string        `json:"missing_dimensions,omitempty"`
	Dimensions        []CostDimension `json:"dimensions,omitempty"`
}

type AssistantNodeMetadata struct {
	ModelResolution *ModelResolutionMetadata `json:"model_resolution,omitempty"`
	NormalizedUsage *NormalizedUsage         `json:"normalized_usage,omitempty"`
	PricingSnapshot *PricingSnapshot         `json:"pricing_snapshot,omitempty"`
	ProviderCost    *ProviderCost            `json:"provider_cost,omitempty"`
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
