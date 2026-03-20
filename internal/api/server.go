// Package api provides an HTTP API server for langdag.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"langdag.com/langdag/internal/config"
	"langdag.com/langdag/internal/conversation"
	"langdag.com/langdag/internal/provider"
	"langdag.com/langdag/internal/provider/anthropic"
	geminiprovider "langdag.com/langdag/internal/provider/gemini"
	mockprovider "langdag.com/langdag/internal/provider/mock"
	openaiprovider "langdag.com/langdag/internal/provider/openai"
	"langdag.com/langdag/internal/storage/sqlite"
)

// Server represents the HTTP API server.
type Server struct {
	httpServer *http.Server
	store      *sqlite.SQLiteStorage
	convMgr    *conversation.Manager
	apiKey     string
}

// Config holds server configuration.
type Config struct {
	Addr   string
	APIKey string // Optional API key for authentication
}

// New creates a new API server.
func New(cfg *Config, appConfig *config.Config) (*Server, error) {
	ctx := context.Background()

	// Initialize storage
	storagePath := appConfig.Storage.Path
	if storagePath == "./langdag.db" {
		storagePath = config.GetDefaultStoragePath()
	}

	if err := config.EnsureStorageDir(storagePath); err != nil {
		return nil, err
	}

	store, err := sqlite.New(storagePath)
	if err != nil {
		return nil, err
	}

	if err := store.Init(ctx); err != nil {
		store.Close()
		return nil, err
	}

	// Create provider (may return a Router when routing is configured)
	prov, err := createProvider(ctx, appConfig)
	if err != nil {
		store.Close()
		return nil, err
	}

	// Create managers
	convMgr := conversation.NewManager(store, prov)

	s := &Server{
		store:   store,
		convMgr: convMgr,
		apiKey:  cfg.APIKey,
	}

	// Setup routes
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Prompt endpoints
	mux.HandleFunc("POST /prompt", s.authMiddleware(s.handlePrompt))
	mux.HandleFunc("POST /nodes/{id}/prompt", s.authMiddleware(s.handleNodePrompt))

	// Node endpoints
	mux.HandleFunc("GET /nodes", s.authMiddleware(s.handleListNodes))
	mux.HandleFunc("GET /nodes/{id}", s.authMiddleware(s.handleGetNode))
	mux.HandleFunc("GET /nodes/{id}/tree", s.authMiddleware(s.handleGetTree))
	mux.HandleFunc("DELETE /nodes/{id}", s.authMiddleware(s.handleDeleteNode))

	// Alias endpoints
	mux.HandleFunc("PUT /nodes/{id}/aliases/{alias}", s.authMiddleware(s.handleCreateAlias))
	mux.HandleFunc("GET /nodes/{id}/aliases", s.authMiddleware(s.handleListAliases))
	mux.HandleFunc("DELETE /aliases/{alias}", s.authMiddleware(s.handleDeleteAlias))

	s.httpServer = &http.Server{
		Addr:         cfg.Addr,
		Handler:      s.corsMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Disable for SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	return s, nil
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	log.Printf("Starting API server on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.store.Close()
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// authMiddleware checks for API key authentication if configured.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				auth = r.Header.Get("X-API-Key")
			} else {
				auth = strings.TrimPrefix(auth, "Bearer ")
			}

			if auth != s.apiKey {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next(w, r)
	}
}

// corsMiddleware adds CORS headers.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// decodeJSON decodes JSON from the request body.
func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// parseRetryConfig parses a config.RetryConfig into a provider.RetryConfig,
// falling back to the global retry config for unset fields.
func parseRetryConfig(rc config.RetryConfig, global provider.RetryConfig) provider.RetryConfig {
	cfg := global
	if rc.MaxRetries > 0 {
		cfg.MaxRetries = rc.MaxRetries
	}
	if rc.BaseDelay != "" {
		if d, err := time.ParseDuration(rc.BaseDelay); err == nil {
			cfg.BaseDelay = d
		}
	}
	if rc.MaxDelay != "" {
		if d, err := time.ParseDuration(rc.MaxDelay); err == nil {
			cfg.MaxDelay = d
		}
	}
	return cfg
}

// providerFactory is a function that creates a provider.
type providerFactory func(ctx context.Context, appConfig *config.Config) (provider.Provider, error)

// providerRegistry maps provider names to their factory functions.
var providerRegistry = map[string]providerFactory{
	"anthropic": func(_ context.Context, c *config.Config) (provider.Provider, error) {
		if c.Providers.Anthropic.APIKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
		}
		return anthropic.New(c.Providers.Anthropic.APIKey), nil
	},
	"anthropic-vertex": func(ctx context.Context, c *config.Config) (provider.Provider, error) {
		vc := c.Providers.AnthropicVertex
		if vc.ProjectID == "" || vc.Region == "" {
			return nil, fmt.Errorf("VERTEX_PROJECT_ID and VERTEX_REGION must be set for anthropic-vertex")
		}
		return anthropic.NewVertex(ctx, vc.Region, vc.ProjectID)
	},
	"anthropic-bedrock": func(ctx context.Context, c *config.Config) (provider.Provider, error) {
		return anthropic.NewBedrock(ctx)
	},
	"openai": func(_ context.Context, c *config.Config) (provider.Provider, error) {
		if c.Providers.OpenAI.APIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY not set")
		}
		return openaiprovider.New(c.Providers.OpenAI.APIKey, c.Providers.OpenAI.BaseURL), nil
	},
	"openai-azure": func(_ context.Context, c *config.Config) (provider.Provider, error) {
		ac := c.Providers.OpenAIAzure
		if ac.APIKey == "" || ac.Endpoint == "" {
			return nil, fmt.Errorf("AZURE_OPENAI_API_KEY and AZURE_OPENAI_ENDPOINT must be set for openai-azure")
		}
		return openaiprovider.NewAzure(ac.APIKey, ac.Endpoint, ac.APIVersion), nil
	},
	"grok": func(_ context.Context, c *config.Config) (provider.Provider, error) {
		if c.Providers.Grok.APIKey == "" {
			return nil, fmt.Errorf("XAI_API_KEY not set")
		}
		return openaiprovider.NewGrok(c.Providers.Grok.APIKey, c.Providers.Grok.BaseURL), nil
	},
	"ollama": func(_ context.Context, c *config.Config) (provider.Provider, error) {
		baseURL := "http://localhost:11434"
		if c.Providers.Ollama.BaseURL != "" {
			baseURL = c.Providers.Ollama.BaseURL
		}
		if c.Providers.Ollama.APIKey != "" {
			return openaiprovider.NewOllamaWithAPIKey(baseURL, c.Providers.Ollama.APIKey), nil
		}
		return openaiprovider.NewOllama(baseURL), nil
	},
	"gemini": func(_ context.Context, c *config.Config) (provider.Provider, error) {
		if c.Providers.Gemini.APIKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY not set")
		}
		return geminiprovider.New(c.Providers.Gemini.APIKey), nil
	},
	"gemini-vertex": func(ctx context.Context, c *config.Config) (provider.Provider, error) {
		vc := c.Providers.GeminiVertex
		if vc.ProjectID == "" || vc.Region == "" {
			return nil, fmt.Errorf("VERTEX_PROJECT_ID and VERTEX_REGION must be set for gemini-vertex")
		}
		return geminiprovider.NewVertex(ctx, vc.ProjectID, vc.Region)
	},
	"mock": func(_ context.Context, c *config.Config) (provider.Provider, error) {
		cfg := mockprovider.Config{
			Mode:          c.Providers.Mock.Mode,
			FixedResponse: c.Providers.Mock.FixedResponse,
		}
		if c.Providers.Mock.Delay != "" {
			d, err := time.ParseDuration(c.Providers.Mock.Delay)
			if err != nil {
				return nil, fmt.Errorf("invalid mock delay: %w", err)
			}
			cfg.Delay = d
		}
		if c.Providers.Mock.ChunkDelay != "" {
			d, err := time.ParseDuration(c.Providers.Mock.ChunkDelay)
			if err != nil {
				return nil, fmt.Errorf("invalid mock chunk_delay: %w", err)
			}
			cfg.ChunkDelay = d
		}
		return mockprovider.New(cfg), nil
	},
}

// createProvider creates the LLM provider based on configuration.
// When routing config is present, it builds a Router with weighted selection
// and fallback. Otherwise, it creates a single provider with global retry.
func createProvider(ctx context.Context, appConfig *config.Config) (provider.Provider, error) {
	globalRetry := parseRetryConfig(appConfig.Retry, provider.DefaultRetryConfig())

	// If routing is configured, build a Router
	if len(appConfig.Providers.Routing) > 0 {
		return createRouter(ctx, appConfig, globalRetry)
	}

	// Single-provider mode (backward compatible)
	name := appConfig.Providers.Default
	factory, ok := providerRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}

	prov, err := factory(ctx, appConfig)
	if err != nil {
		return nil, err
	}

	log.Printf("Using provider: %s", name)
	return provider.WithRetry(prov, globalRetry), nil
}

// createRouter builds a Router from the routing and fallback config.
func createRouter(ctx context.Context, appConfig *config.Config, globalRetry provider.RetryConfig) (provider.Provider, error) {
	// Build a cache of providers by name (so we don't create duplicates)
	providerCache := map[string]provider.Provider{}

	getOrCreate := func(name string) (provider.Provider, error) {
		if p, ok := providerCache[name]; ok {
			return p, nil
		}
		factory, ok := providerRegistry[name]
		if !ok {
			return nil, fmt.Errorf("unknown provider in routing config: %s", name)
		}
		p, err := factory(ctx, appConfig)
		if err != nil {
			return nil, nil // silently drop unavailable providers
		}
		providerCache[name] = p
		return p, nil
	}

	// Build routing entries
	var entries []provider.RouteEntry
	for _, re := range appConfig.Providers.Routing {
		p, err := getOrCreate(re.Provider)
		if err != nil {
			return nil, err
		}
		if p == nil {
			log.Printf("Skipping unavailable provider in routing: %s", re.Provider)
			continue
		}
		// Wrap with per-provider retry
		retryCfg := parseRetryConfig(re.Retry, globalRetry)
		wrapped := provider.WithRetry(p, retryCfg)
		entries = append(entries, provider.RouteEntry{Provider: wrapped, Weight: re.Weight})
		log.Printf("Routing: %s (weight=%d)", re.Provider, re.Weight)
	}

	// Build fallback chain
	var fallbackProviders []provider.Provider
	for _, name := range appConfig.Providers.FallbackOrder {
		p, err := getOrCreate(name)
		if err != nil {
			return nil, err
		}
		if p == nil {
			log.Printf("Skipping unavailable provider in fallback: %s", name)
			continue
		}
		wrapped := provider.WithRetry(p, globalRetry)
		fallbackProviders = append(fallbackProviders, wrapped)
		log.Printf("Fallback: %s", name)
	}

	return provider.NewRouter(entries, fallbackProviders)
}
