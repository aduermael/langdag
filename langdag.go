// Package langdag provides a Go library for managing AI agent conversations
// with tree-structured storage and multi-provider LLM routing.
package langdag

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"langdag.com/langdag/internal/conversation"
	"langdag.com/langdag/internal/models"
	internalprovider "langdag.com/langdag/internal/provider"
	anthropicprovider "langdag.com/langdag/internal/provider/anthropic"
	gemmaprovider "langdag.com/langdag/internal/provider/gemma"
	geminiprovider "langdag.com/langdag/internal/provider/gemini"
	openaiprovider "langdag.com/langdag/internal/provider/openai"
	internalstorage "langdag.com/langdag/internal/storage"
	"langdag.com/langdag/internal/storage/sqlite"
	"langdag.com/langdag/types"
)

// Storage is the interface for persisting conversation nodes.
// It is re-exported here so that external consumers can use the return value
// of Client.Storage() without importing an internal package.
type Storage = internalstorage.Storage

// Provider is the interface for LLM providers.
// It is re-exported here so that external consumers can use the return value
// of Client.Provider() without importing an internal package.
type Provider = internalprovider.Provider

// ModelPricing contains pricing and capability information for a model.
type ModelPricing = models.ModelPricing

// ModelCatalog contains model pricing and capability information organized by provider.
type ModelCatalog = models.Catalog

// Config holds all configuration for the langdag client.
type Config struct {
	// StoragePath is the path to the SQLite database file.
	// Defaults to "$HOME/.config/langdag/langdag.db"
	StoragePath string

	// Provider is the default LLM provider to use.
	// Valid values: "anthropic", "openai", "gemini", "gemma", "grok", "ollama",
	// "anthropic-vertex", "anthropic-bedrock", "openai-azure", "gemini-vertex"
	// Defaults to "anthropic"
	Provider string

	// APIKeys maps provider names to their API keys.
	// Keys: "anthropic", "openai", "gemini", "gemma"
	APIKeys map[string]string

	// AnthropicConfig holds Anthropic-specific config (optional base URL override).
	AnthropicConfig *AnthropicConfig

	// OpenAIConfig holds OpenAI-specific config.
	OpenAIConfig *OpenAIConfig

	// GeminiConfig holds Gemini-specific config.
	GeminiConfig *GeminiConfig

	// GemmaConfig holds Gemma-specific config.
	GemmaConfig *GemmaConfig

	// GrokConfig holds Grok (xAI)-specific config.
	GrokConfig *GrokConfig

	// AzureOpenAIConfig holds Azure OpenAI-specific config.
	AzureOpenAIConfig *AzureOpenAIConfig

	// VertexConfig holds Google Vertex AI config.
	VertexConfig *VertexConfig

	// BedrockConfig holds AWS Bedrock config.
	BedrockConfig *BedrockConfig

	// OllamaConfig holds Ollama-specific config (local LLM server).
	OllamaConfig *OllamaConfig

	// Routing configures multi-provider routing (optional).
	Routing []RoutingEntry

	// FallbackOrder specifies provider fallback order (optional).
	FallbackOrder []string

	// RetryConfig configures retry behavior.
	RetryConfig *RetryConfig
}

// AnthropicConfig holds Anthropic-specific configuration.
type AnthropicConfig struct {
	BaseURL string
}

// OpenAIConfig holds OpenAI-specific configuration.
type OpenAIConfig struct {
	BaseURL string
}

// GeminiConfig holds Gemini-specific configuration.
type GeminiConfig struct {
	BaseURL string
}

// GemmaConfig holds Gemma-specific configuration. Gemma uses the same
// Google AI Studio endpoint as Gemini, but the key and routing are kept
// independent so the two providers can be configured separately.
type GemmaConfig struct {
	BaseURL string
}

// AzureOpenAIConfig holds Azure OpenAI-specific configuration.
type AzureOpenAIConfig struct {
	Endpoint   string
	APIVersion string
	APIKey     string
}

// VertexConfig holds Google Vertex AI configuration.
type VertexConfig struct {
	ProjectID string
	Region    string
}

// BedrockConfig holds AWS Bedrock configuration.
type BedrockConfig struct {
	Region string
}

// OllamaConfig holds Ollama-specific configuration.
// Ollama is a local LLM server that provides an OpenAI-compatible API.
type OllamaConfig struct {
	// BaseURL is the Ollama server address (e.g., "http://localhost:11434" or "http://100.93.184.1:11434")
	BaseURL string
}

// GrokConfig holds Grok (xAI)-specific configuration.
type GrokConfig struct {
	BaseURL string
}

// RoutingEntry configures a single provider entry in the routing table.
type RoutingEntry struct {
	Provider string
	Weight   int
	Retry    *RetryConfig
}

// RetryConfig configures retry behavior for LLM provider calls.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// Client is the main langdag client for managing AI conversations.
//
// A Client is safe for concurrent use by multiple goroutines. The underlying
// storage (SQLite with WAL mode and busy_timeout) and providers (stateless HTTP
// clients) are themselves concurrent-safe, and each call to Prompt or PromptFrom
// returns an independent PromptResult with its own streaming channel.
type Client struct {
	store   internalstorage.Storage
	prov    internalprovider.Provider
	convMgr *conversation.Manager
}

// New creates a new langdag client with the given configuration.
func New(cfg Config) (*Client, error) {
	ctx := context.Background()

	// Resolve storage path
	storagePath := cfg.StoragePath
	if storagePath == "" {
		storagePath = defaultStoragePath()
	}

	// Ensure the directory for the storage file exists
	if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil {
		return nil, fmt.Errorf("langdag: failed to create storage directory: %w", err)
	}

	// Initialize SQLite storage
	store, err := sqlite.New(storagePath)
	if err != nil {
		return nil, fmt.Errorf("langdag: failed to open storage: %w", err)
	}

	if err := store.Init(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("langdag: failed to initialize storage: %w", err)
	}

	// Build the provider
	prov, err := buildProvider(ctx, cfg)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("langdag: failed to create provider: %w", err)
	}

	convMgr := conversation.NewManager(store, prov)

	return &Client{
		store:   store,
		prov:    prov,
		convMgr: convMgr,
	}, nil
}

// NewWithDeps creates a Client from pre-built dependencies.
// Useful for testing or custom integrations where the caller has already
// constructed a Storage and Provider.
func NewWithDeps(store Storage, prov Provider) *Client {
	return &Client{
		store:   store,
		prov:    prov,
		convMgr: conversation.NewManager(store, prov),
	}
}

// Close releases all resources held by the client.
func (c *Client) Close() error {
	return c.store.Close()
}

// Storage returns the underlying storage interface for advanced use cases.
func (c *Client) Storage() Storage {
	return c.store
}

// Provider returns the underlying provider for advanced use cases.
func (c *Client) Provider() Provider {
	return c.prov
}

// PromptOption configures a prompt request.
type PromptOption func(*promptOptions)

type promptOptions struct {
	model                string
	systemPrompt         string
	maxTokens            int
	maxOutputGroupTokens int
	maxTurns             int
	tools                []types.ToolDefinition
	think                *bool
}

// WithModel sets the model for the prompt.
func WithModel(model string) PromptOption {
	return func(o *promptOptions) {
		o.model = model
	}
}

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(prompt string) PromptOption {
	return func(o *promptOptions) {
		o.systemPrompt = prompt
	}
}

// WithMaxTokens sets the max tokens for the response.
func WithMaxTokens(n int) PromptOption {
	return func(o *promptOptions) {
		o.maxTokens = n
	}
}

// WithMaxOutputGroupTokens sets the maximum total output tokens across all
// continuation calls in an output group. When a response hits max_tokens and
// is continued, langdag tracks cumulative output tokens; if they exceed this
// budget the continuation stops. A value of 0 (the default) means 4× the
// per-call max_tokens.
func WithMaxOutputGroupTokens(n int) PromptOption {
	return func(o *promptOptions) {
		o.maxOutputGroupTokens = n
	}
}

// WithMaxTurns sets the maximum number of LLM round-trips for a single
// Prompt/PromptFrom call. Since langdag does not have a built-in tool-use
// loop, the value is exposed on the PromptResult so the caller can
// decrement and check it in their own multi-turn loop.
// A value of 0 (the default) means unlimited.
func WithMaxTurns(n int) PromptOption {
	return func(o *promptOptions) {
		o.maxTurns = n
	}
}

// WithTools sets the tool definitions for the prompt.
// When tools are provided, the LLM may respond with tool_use content blocks.
func WithTools(tools []types.ToolDefinition) PromptOption {
	return func(o *promptOptions) {
		o.tools = tools
	}
}

// WithThink controls whether the model should use extended thinking.
// true = enable thinking, false = disable thinking. Omitting this option
// leaves the decision to the provider/model default.
func WithThink(enabled bool) PromptOption {
	return func(o *promptOptions) {
		o.think = &enabled
	}
}

// PromptResult holds the result of a prompt call.
//
// The NodeID and Content fields are written by a background goroutine as the
// stream is consumed. Reading them directly before the stream is fully drained
// is a data race. Use the GetNodeID and GetContent accessor methods for
// concurrent-safe access at any time, or read the fields only after the Stream
// channel has been fully drained (closed).
type PromptResult struct {
	// NodeID is the ID of the saved assistant node (set when streaming completes,
	// or immediately for non-streaming use when the stream is consumed).
	NodeID string

	// Content is the full response text (empty until streaming completes).
	Content string

	// Stream is the streaming channel. Range over it to receive chunks.
	// It is never nil; even for simple use cases, consumers should drain it.
	Stream <-chan StreamChunk

	// MaxTurns is the maximum number of LLM round-trips the caller should
	// allow for this conversation. 0 means unlimited. Since langdag does not
	// have a built-in tool-use loop, the caller can use this value to enforce
	// a turn budget in their own multi-turn loop.
	MaxTurns int

	// mu protects concurrent writes to NodeID and Content from the background
	// goroutine in buildResult.
	mu sync.Mutex
}

// GetNodeID returns the node ID in a concurrent-safe manner.
// The value is only meaningful after the stream has delivered a Done chunk.
func (r *PromptResult) GetNodeID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.NodeID
}

// GetContent returns the full response content in a concurrent-safe manner.
// The value is only meaningful after the stream has been fully drained.
func (r *PromptResult) GetContent() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Content
}

// StreamChunk is a piece of a streaming response.
type StreamChunk struct {
	// Content is the incremental text content for this chunk.
	Content string

	// ContentBlock is set for content_done events (e.g. tool_use blocks).
	ContentBlock *types.ContentBlock

	// Done indicates the stream has completed.
	Done bool

	// Error holds any error that occurred during streaming.
	Error error

	// NodeID is set when Done=true and the assistant node has been saved to storage.
	NodeID string

	// StopReason is the reason the LLM stopped generating (e.g. "end_turn", "tool_use").
	// Set when Done=true.
	StopReason string
}

// Prompt starts a new conversation with the given message.
// Returns a PromptResult with the streaming response.
func (c *Client) Prompt(ctx context.Context, message string, opts ...PromptOption) (*PromptResult, error) {
	o := applyOptions(opts)
	events, err := c.convMgr.Prompt(ctx, message, o.model, o.systemPrompt, o.tools, o.think, o.maxTokens, o.maxOutputGroupTokens)
	if err != nil {
		return nil, err
	}
	result := buildResult(events)
	result.MaxTurns = o.maxTurns
	return result, nil
}

// PromptFrom continues a conversation from an existing node.
func (c *Client) PromptFrom(ctx context.Context, nodeID string, message string, opts ...PromptOption) (*PromptResult, error) {
	o := applyOptions(opts)
	events, err := c.convMgr.PromptFrom(ctx, nodeID, message, o.model, o.tools, o.think, o.maxTokens, o.maxOutputGroupTokens)
	if err != nil {
		return nil, err
	}
	result := buildResult(events)
	result.MaxTurns = o.maxTurns
	return result, nil
}

// ListConversations returns all root conversation nodes.
func (c *Client) ListConversations(ctx context.Context) ([]*types.Node, error) {
	return c.convMgr.ListRoots(ctx)
}

// GetNode returns a node by ID or ID prefix.
func (c *Client) GetNode(ctx context.Context, id string) (*types.Node, error) {
	return c.convMgr.ResolveNode(ctx, id)
}

// GetSubtree returns a node and all its descendants.
func (c *Client) GetSubtree(ctx context.Context, id string) ([]*types.Node, error) {
	node, err := c.convMgr.ResolveNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("langdag: node not found: %s", id)
	}
	return c.convMgr.GetSubtree(ctx, node.ID)
}

// GetAncestors returns all ancestors of a node (the conversation history leading to it).
func (c *Client) GetAncestors(ctx context.Context, id string) ([]*types.Node, error) {
	node, err := c.convMgr.ResolveNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("langdag: node not found: %s", id)
	}
	return c.store.GetAncestors(ctx, node.ID)
}

// DeleteNode deletes a node and all its descendants.
func (c *Client) DeleteNode(ctx context.Context, id string) error {
	node, err := c.convMgr.ResolveNode(ctx, id)
	if err != nil {
		return err
	}
	if node == nil {
		return fmt.Errorf("langdag: node not found: %s", id)
	}
	return c.convMgr.DeleteNode(ctx, node.ID)
}

// applyOptions applies prompt options and returns the resulting promptOptions.
func applyOptions(opts []PromptOption) *promptOptions {
	o := &promptOptions{
		model: "claude-sonnet-4-20250514",
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// buildResult converts a channel of types.StreamEvent into a PromptResult with a StreamChunk channel.
// The returned PromptResult.Content and PromptResult.NodeID are populated once the stream completes
// (i.e., after the Stream channel is drained).
func buildResult(events <-chan types.StreamEvent) *PromptResult {
	ch := make(chan StreamChunk, 100)
	result := &PromptResult{Stream: ch}

	go func() {
		defer close(ch)
		var accumulated string
		var stopReason string
		var terminated bool
		for event := range events {
			switch event.Type {
			case types.StreamEventDelta:
				accumulated += event.Content
				ch <- StreamChunk{Content: event.Content}
			case types.StreamEventContentDone:
				ch <- StreamChunk{ContentBlock: event.ContentBlock}
			case types.StreamEventDone:
				if event.Response != nil {
					stopReason = event.Response.StopReason
				}
			case types.StreamEventError:
				ch <- StreamChunk{Error: event.Error, Done: true}
				terminated = true
				return
			case types.StreamEventNodeSaved:
				result.mu.Lock()
				result.NodeID = event.NodeID
				result.Content = accumulated
				result.mu.Unlock()
				ch <- StreamChunk{Done: true, NodeID: event.NodeID, StopReason: stopReason}
				terminated = true
			}
		}
		// If the stream ended without a NodeSaved or Error event (e.g. provider
		// closed the channel early or returned an empty stream), send a Done
		// chunk so consumers never hang waiting for one.
		if !terminated {
			result.mu.Lock()
			result.Content = accumulated
			result.mu.Unlock()
			ch <- StreamChunk{
				Done:       true,
				Error:      fmt.Errorf("stream ended without completion"),
				StopReason: stopReason,
			}
		}
	}()

	return result
}

// defaultStoragePath returns the default path for the SQLite database.
func defaultStoragePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./langdag.db"
	}
	return filepath.Join(homeDir, ".config", "langdag", "langdag.db")
}

// buildProvider creates the appropriate provider based on configuration.
func buildProvider(ctx context.Context, cfg Config) (internalprovider.Provider, error) {
	// Resolve global retry config
	globalRetry := resolveRetryConfig(cfg.RetryConfig)

	// If routing is configured, build a Router
	if len(cfg.Routing) > 0 {
		return buildRouter(ctx, cfg, globalRetry)
	}

	// Single-provider mode
	providerName := cfg.Provider
	if providerName == "" {
		providerName = "anthropic"
	}

	prov, err := createSingleProvider(ctx, providerName, cfg)
	if err != nil {
		return nil, err
	}

	log.Printf("langdag: using provider: %s", providerName)
	return internalprovider.WithRetry(internalprovider.WithServerToolFilter(prov), globalRetry), nil
}

// createSingleProvider constructs a single provider by name.
func createSingleProvider(ctx context.Context, name string, cfg Config) (internalprovider.Provider, error) {
	switch name {
	case "anthropic":
		apiKey := cfg.APIKeys["anthropic"]
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("langdag: ANTHROPIC_API_KEY not set")
		}
		return anthropicprovider.New(apiKey), nil

	case "openai":
		apiKey := cfg.APIKeys["openai"]
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("langdag: OPENAI_API_KEY not set")
		}
		baseURL := ""
		if cfg.OpenAIConfig != nil {
			baseURL = cfg.OpenAIConfig.BaseURL
		}
		if baseURL == "" {
			baseURL = os.Getenv("OPENAI_BASE_URL")
		}
		return openaiprovider.New(apiKey, baseURL), nil

	case "gemini":
		apiKey := cfg.APIKeys["gemini"]
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("langdag: GEMINI_API_KEY not set")
		}
		return geminiprovider.New(apiKey), nil

	case "gemma":
		apiKey := cfg.APIKeys["gemma"]
		if apiKey == "" {
			apiKey = os.Getenv("GEMMA_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("langdag: GEMMA_API_KEY not set")
		}
		return gemmaprovider.New(apiKey), nil

	case "anthropic-vertex":
		vc := cfg.VertexConfig
		if vc == nil {
			return nil, fmt.Errorf("langdag: VertexConfig must be set for anthropic-vertex provider")
		}
		if vc.ProjectID == "" || vc.Region == "" {
			return nil, fmt.Errorf("langdag: VertexConfig.ProjectID and VertexConfig.Region must be set for anthropic-vertex")
		}
		return anthropicprovider.NewVertex(ctx, vc.Region, vc.ProjectID)

	case "anthropic-bedrock":
		return anthropicprovider.NewBedrock(ctx)

	case "openai-azure":
		ac := cfg.AzureOpenAIConfig
		if ac == nil {
			return nil, fmt.Errorf("langdag: AzureOpenAIConfig must be set for openai-azure provider")
		}
		if ac.APIKey == "" || ac.Endpoint == "" {
			return nil, fmt.Errorf("langdag: AzureOpenAIConfig.APIKey and AzureOpenAIConfig.Endpoint must be set for openai-azure")
		}
		return openaiprovider.NewAzure(ac.APIKey, ac.Endpoint, ac.APIVersion), nil

	case "grok":
		apiKey := cfg.APIKeys["grok"]
		if apiKey == "" {
			apiKey = os.Getenv("XAI_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("langdag: XAI_API_KEY not set")
		}
		baseURL := ""
		if cfg.GrokConfig != nil {
			baseURL = cfg.GrokConfig.BaseURL
		}
		if baseURL == "" {
			baseURL = os.Getenv("XAI_BASE_URL")
		}
		return openaiprovider.NewGrok(apiKey, baseURL), nil

	case "gemini-vertex":
		vc := cfg.VertexConfig
		if vc == nil {
			return nil, fmt.Errorf("langdag: VertexConfig must be set for gemini-vertex provider")
		}
		if vc.ProjectID == "" || vc.Region == "" {
			return nil, fmt.Errorf("langdag: VertexConfig.ProjectID and VertexConfig.Region must be set for gemini-vertex")
		}
		return geminiprovider.NewVertex(ctx, vc.ProjectID, vc.Region)

	case "ollama":
		baseURL := "http://localhost:11434"
		if cfg.OllamaConfig != nil && cfg.OllamaConfig.BaseURL != "" {
			baseURL = cfg.OllamaConfig.BaseURL
		}
		return openaiprovider.NewOllama(baseURL), nil

	default:
		return nil, fmt.Errorf("langdag: unknown provider: %s", name)
	}
}

// buildRouter creates a Router from routing and fallback configuration.
func buildRouter(ctx context.Context, cfg Config, globalRetry internalprovider.RetryConfig) (internalprovider.Provider, error) {
	providerCache := map[string]internalprovider.Provider{}

	getOrCreate := func(name string) (internalprovider.Provider, error) {
		if p, ok := providerCache[name]; ok {
			return p, nil
		}
		p, err := createSingleProvider(ctx, name, cfg)
		if err != nil {
			// Silently drop unavailable providers in routing (consistent with api/server.go)
			log.Printf("langdag: skipping unavailable provider %q: %v", name, err)
			return nil, nil //nolint:nilerr
		}
		providerCache[name] = p
		return p, nil
	}

	// Build routing entries
	var entries []internalprovider.RouteEntry
	for _, re := range cfg.Routing {
		p, err := getOrCreate(re.Provider)
		if err != nil {
			return nil, err
		}
		if p == nil {
			continue
		}
		entryCfg := resolveRetryConfig(re.Retry)
		wrapped := internalprovider.WithRetry(internalprovider.WithServerToolFilter(p), entryCfg)
		entries = append(entries, internalprovider.RouteEntry{Provider: wrapped, Weight: re.Weight})
		log.Printf("langdag: routing: %s (weight=%d)", re.Provider, re.Weight)
	}

	// Build fallback chain
	var fallbackProviders []internalprovider.Provider
	for _, name := range cfg.FallbackOrder {
		p, err := getOrCreate(name)
		if err != nil {
			return nil, err
		}
		if p == nil {
			continue
		}
		wrapped := internalprovider.WithRetry(internalprovider.WithServerToolFilter(p), globalRetry)
		fallbackProviders = append(fallbackProviders, wrapped)
		log.Printf("langdag: fallback: %s", name)
	}

	return internalprovider.NewRouter(entries, fallbackProviders)
}

// DefaultModelCatalog returns the model catalog embedded with the library.
// It contains model names, pricing (per 1M tokens), context window sizes,
// and max output tokens for all supported providers.
func DefaultModelCatalog() (*ModelCatalog, error) {
	return models.DefaultCatalog()
}

// LoadModelCatalog loads the model catalog from a cache file, falling back to
// the embedded default if the file does not exist or is invalid JSON.
func LoadModelCatalog(cachePath string) (*ModelCatalog, error) {
	return models.LoadCatalog(cachePath)
}

// FetchModelCatalog fetches the latest model catalog from official provider
// documentation pages (OpenAI, Anthropic, Google, xAI).
// This does not require any API keys.
//
// If cachePath is non-empty, the fetched catalog is saved to that path
// so it can be loaded with LoadModelCatalog in future sessions.
func FetchModelCatalog(ctx context.Context, cachePath string) (*ModelCatalog, error) {
	catalog, err := models.FetchLatest(ctx)
	if err != nil {
		return nil, err
	}
	if cachePath != "" {
		if saveErr := models.SaveCatalog(catalog, cachePath); saveErr != nil {
			return catalog, fmt.Errorf("langdag: catalog fetched but save failed: %w", saveErr)
		}
	}
	return catalog, nil
}

// resolveRetryConfig converts a *RetryConfig to internalprovider.RetryConfig,
// falling back to the default when nil.
func resolveRetryConfig(rc *RetryConfig) internalprovider.RetryConfig {
	defaults := internalprovider.DefaultRetryConfig()
	if rc == nil {
		return defaults
	}
	cfg := defaults
	if rc.MaxRetries > 0 {
		cfg.MaxRetries = rc.MaxRetries
	}
	if rc.BaseDelay > 0 {
		cfg.BaseDelay = rc.BaseDelay
	}
	if rc.MaxDelay > 0 {
		cfg.MaxDelay = rc.MaxDelay
	}
	return cfg
}
