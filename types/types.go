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
	Type string `json:"type"` // "text", "image", "document", "tool_use", "tool_result"

	// For text blocks
	Text string `json:"text,omitempty"`

	// For image/document blocks
	MediaType string `json:"media_type,omitempty"` // e.g. "image/png", "application/pdf"
	Data      string `json:"data,omitempty"`       // base64-encoded content
	URL       string `json:"url,omitempty"`        // URL source

	// For tool_use blocks
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// For tool_result blocks
	ToolUseID   string          `json:"tool_use_id,omitempty"`
	Content     string          `json:"content,omitempty"`
	ContentJSON json.RawMessage `json:"content_json,omitempty"` // structured tool result (takes priority over Content when set)
	IsError     bool            `json:"is_error,omitempty"`
	DurationMs  int             `json:"duration_ms,omitempty"` // tool execution time in ms

	// ProviderData holds opaque provider-specific data that must survive
	// round-trips (e.g. Gemini thought signatures for multi-turn tool use).
	// Providers store and retrieve structured data here; other providers ignore it.
	ProviderData json.RawMessage `json:"provider_data,omitempty"`
}

// ToolResultContent returns the tool result content as a JSON value.
// If ContentJSON is set, it is returned directly (structured result).
// Otherwise, Content is marshaled as a JSON string.
// Returns nil if both ContentJSON and Content are empty.
func (b *ContentBlock) ToolResultContent() json.RawMessage {
	if len(b.ContentJSON) > 0 {
		return b.ContentJSON
	}
	if b.Content == "" {
		return nil
	}
	// Marshal the plain string as a JSON string value.
	data, _ := json.Marshal(b.Content)
	return json.RawMessage(data)
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
)

// Node represents a node in the conversation/workflow tree.
// Root nodes (ParentID == "") define the start of a tree and carry
// metadata like Title and SystemPrompt.
type Node struct {
	ID       string   `json:"id"`
	ParentID string   `json:"parent_id,omitempty"`
	RootID   string   `json:"root_id,omitempty"`
	Sequence int      `json:"sequence"`
	NodeType NodeType `json:"node_type"`
	Content  string   `json:"content"`

	// LLM execution metadata (on assistant nodes)
	Provider            string `json:"provider,omitempty"`
	Model               string `json:"model,omitempty"`
	TokensIn            int    `json:"tokens_in,omitempty"`
	TokensOut           int    `json:"tokens_out,omitempty"`
	TokensCacheRead     int    `json:"tokens_cache_read,omitempty"`
	TokensCacheCreation int    `json:"tokens_cache_creation,omitempty"`
	TokensReasoning     int    `json:"tokens_reasoning,omitempty"`
	LatencyMs           int    `json:"latency_ms,omitempty"`
	StopReason          string `json:"stop_reason,omitempty"`
	OutputGroupID       string `json:"output_group_id,omitempty"`
	Status              string `json:"status,omitempty"`

	// Root node metadata (empty on non-root nodes)
	Title        string `json:"title,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`

	CreatedAt time.Time       `json:"created_at"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// Tree represents a tree of nodes rooted at a specific node.
type Tree struct {
	Root  *Node  `json:"root"`
	Nodes []Node `json:"nodes"`
}

// ToolDefinition represents a tool that can be used in a completion request.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ServerToolWebSearch is the standardized name for web search across providers.
// Use this as the Name in a ToolDefinition to enable the provider's built-in web search.
// Each provider maps the name to its native equivalent:
//   - Anthropic: web_search_20250305
//   - OpenAI:    web_search_preview
//   - Gemini:    google_search
//   - Grok:      web_search (Responses API)
const ServerToolWebSearch = "web_search"

// IsClientTool reports whether t is a client-side (user-defined function) tool.
// A tool is client-side if it has an InputSchema — the client declares its
// parameters so the LLM knows how to call it. Tools without an InputSchema
// are treated as server-side tools provided by the LLM platform.
//
// Client-side tools take priority: if you provide an InputSchema for a name
// like "web_search", it will be sent as a function tool rather than the
// provider's built-in web search.
func (t ToolDefinition) IsClientTool() bool {
	return len(t.InputSchema) > 0
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
	Think       *bool            `json:"think,omitempty"` // nil = provider default, true = enable, false = disable
}

// CompletionResponse represents a response from an LLM provider.
type CompletionResponse struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Provider   string         `json:"provider,omitempty"` // Which provider served this request
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

// Usage represents token usage information.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
}

// NormalizedUsage preserves billable usage dimensions across providers. Common
// dimensions are first-class fields for callers that need stable access; any
// provider-specific counters that do not yet have first-class fields belong in
// Dimensions using catalog pricing dimension names.
type NormalizedUsage struct {
	InputTokens              int              `json:"input_tokens,omitempty"`
	OutputTokens             int              `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int              `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int              `json:"cache_creation_input_tokens,omitempty"`
	ReasoningTokens          int              `json:"reasoning_tokens,omitempty"`
	ToolUsePromptTokens      int              `json:"tool_use_prompt_tokens,omitempty"`
	ServiceTier              string           `json:"service_tier,omitempty"`
	Dimensions               map[string]int64 `json:"dimensions,omitempty"`
}

// NormalizedUsageFromUsage adapts the legacy token-only Usage struct into the
// deployment-aware accounting shape.
func NormalizedUsageFromUsage(usage Usage) NormalizedUsage {
	return NormalizedUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		ReasoningTokens:          usage.ReasoningTokens,
	}
}

func (u NormalizedUsage) BillableDimensions() map[string]int64 {
	dimensions := map[string]int64{}
	add := func(name string, value int) {
		if value > 0 {
			dimensions[name] += int64(value)
		}
	}
	add("input_tokens", u.InputTokens)
	add("output_tokens", u.OutputTokens)
	add("cache_read_input_tokens", u.CacheReadInputTokens)
	add("cache_creation_input_tokens", u.CacheCreationInputTokens)
	add("reasoning_tokens", u.ReasoningTokens)
	add("tool_use_prompt_tokens", u.ToolUsePromptTokens)
	for name, value := range u.Dimensions {
		if name != "" && value > 0 {
			dimensions[name] += value
		}
	}
	return dimensions
}

type CostStatus string

const (
	CostStatusKnown   CostStatus = "known"
	CostStatusPartial CostStatus = "partial"
	CostStatusUnknown CostStatus = "unknown"
	CostStatusFree    CostStatus = "free"
)

type CostSource string

const (
	CostSourceCatalog          CostSource = "catalog"
	CostSourceProviderResponse CostSource = "provider_response"
	CostSourceHistorical       CostSource = "historical"
)

// PricingSnapshot is copied onto assistant-node metadata so historical cost
// display does not change when catalog pricing is refreshed later.
type PricingSnapshot struct {
	Status            CostStatus         `json:"status"`
	Currency          string             `json:"currency,omitempty"`
	EffectiveAt       time.Time          `json:"effective_at,omitempty"`
	Source            CostSource         `json:"source,omitempty"`
	RatesPer1M        map[string]float64 `json:"rates_per_1m,omitempty"`
	MissingDimensions []string           `json:"missing_dimensions,omitempty"`
}

type CostDimension struct {
	Name      string  `json:"name"`
	Quantity  int64   `json:"quantity"`
	RatePer1M float64 `json:"rate_per_1m"`
	Cost      float64 `json:"cost"`
}

// CostResult is the structured replacement for float-only cost calculation.
// Unknown pricing is represented explicitly rather than as zero dollars.
type CostResult struct {
	Status            CostStatus      `json:"status"`
	Total             float64         `json:"total,omitempty"`
	Currency          string          `json:"currency,omitempty"`
	Source            CostSource      `json:"source,omitempty"`
	MissingDimensions []string        `json:"missing_dimensions,omitempty"`
	Dimensions        []CostDimension `json:"dimensions,omitempty"`
}

type ProviderCost struct {
	Total    float64         `json:"total"`
	Currency string          `json:"currency"`
	Source   CostSource      `json:"source"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

func CostResultFromProviderCost(cost ProviderCost) CostResult {
	return CostResult{
		Status:   CostStatusKnown,
		Total:    cost.Total,
		Currency: cost.Currency,
		Source:   cost.Source,
	}
}

func ComputeCostFromPricingSnapshot(snapshot PricingSnapshot, usage NormalizedUsage) CostResult {
	if snapshot.Status == CostStatusFree {
		return CostResult{Status: CostStatusFree, Currency: snapshot.Currency, Source: snapshot.Source}
	}
	if snapshot.Status == CostStatusUnknown || len(snapshot.RatesPer1M) == 0 {
		return CostResult{
			Status:            CostStatusUnknown,
			Currency:          snapshot.Currency,
			Source:            snapshot.Source,
			MissingDimensions: append([]string(nil), snapshot.MissingDimensions...),
		}
	}

	result := CostResult{
		Status:   CostStatusKnown,
		Currency: snapshot.Currency,
		Source:   snapshot.Source,
	}
	missing := map[string]bool{}
	for _, dimension := range snapshot.MissingDimensions {
		if dimension != "" {
			missing[dimension] = true
		}
	}
	for dimension, quantity := range usage.BillableDimensions() {
		rate, ok := snapshot.RatesPer1M[dimension]
		if !ok {
			missing[dimension] = true
			continue
		}
		cost := float64(quantity) * rate / 1_000_000
		result.Total += cost
		result.Dimensions = append(result.Dimensions, CostDimension{
			Name:      dimension,
			Quantity:  quantity,
			RatePer1M: rate,
			Cost:      cost,
		})
	}
	if len(missing) > 0 || snapshot.Status == CostStatusPartial {
		result.Status = CostStatusPartial
		result.MissingDimensions = sortedCostDimensionNames(missing)
	}
	return result
}

func sortedCostDimensionNames(values map[string]bool) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

type ModelResolutionMetadata struct {
	CanonicalModelID string `json:"canonical_model_id"`
	OfferingID       string `json:"offering_id"`
	DeploymentID     string `json:"deployment_id"`
	ProviderID       string `json:"provider_id"`
	APIProtocolID    string `json:"api_protocol_id"`
	NativeModelID    string `json:"native_model_id"`
}

// AssistantNodeMetadata is the typed shape stored in Node.Metadata for new
// assistant nodes. Existing provider/model/token columns remain readable for
// old nodes while this metadata carries the deployment-aware identity.
type AssistantNodeMetadata struct {
	ModelResolution *ModelResolutionMetadata `json:"model_resolution,omitempty"`
	NormalizedUsage *NormalizedUsage         `json:"normalized_usage,omitempty"`
	PricingSnapshot *PricingSnapshot         `json:"pricing_snapshot,omitempty"`
	ProviderCost    *ProviderCost            `json:"provider_cost,omitempty"`
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
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ContextWindow int      `json:"context_window"`
	MaxOutput     int      `json:"max_output"`
	ServerTools   []string `json:"server_tools,omitempty"`

	// SupportsFunctionCalling reports whether the model accepts client-side
	// (user-defined) function tools in CompletionRequest.Tools.
	SupportsFunctionCalling bool `json:"supports_function_calling,omitempty"`

	// SupportsExplicitThinkingBudget reports whether the model accepts an
	// explicit thinking budget via CompletionRequest.Think. A model can
	// perform implicit reasoning and still return false here if it rejects
	// explicit budget configuration (e.g. Gemma 4).
	SupportsExplicitThinkingBudget bool `json:"supports_explicit_thinking_budget,omitempty"`
}
