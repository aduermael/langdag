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

	"github.com/langdag/langdag/internal/config"
	"github.com/langdag/langdag/internal/conversation"
	"github.com/langdag/langdag/internal/provider"
	"github.com/langdag/langdag/internal/provider/anthropic"
	mockprovider "github.com/langdag/langdag/internal/provider/mock"
	"github.com/langdag/langdag/internal/storage/sqlite"
	"github.com/langdag/langdag/internal/workflow"
)

// Server represents the HTTP API server.
type Server struct {
	httpServer   *http.Server
	store        *sqlite.SQLiteStorage
	convMgr      *conversation.Manager
	workflowMgr  *workflow.Manager
	apiKey       string
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

	// Create provider
	prov, err := createProvider(appConfig)
	if err != nil {
		store.Close()
		return nil, err
	}

	// Create managers
	convMgr := conversation.NewManager(store, prov)
	workflowMgr := workflow.NewManager(store)

	s := &Server{
		store:       store,
		convMgr:     convMgr,
		workflowMgr: workflowMgr,
		apiKey:      cfg.APIKey,
	}

	// Setup routes
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// DAG endpoints
	mux.HandleFunc("GET /dags", s.authMiddleware(s.handleListDAGs))
	mux.HandleFunc("GET /dags/{id}", s.authMiddleware(s.handleGetDAG))
	mux.HandleFunc("DELETE /dags/{id}", s.authMiddleware(s.handleDeleteDAG))

	// Chat endpoints
	mux.HandleFunc("POST /chat", s.authMiddleware(s.handleNewChat))
	mux.HandleFunc("POST /chat/{id}", s.authMiddleware(s.handleContinueChat))
	mux.HandleFunc("POST /chat/{id}/fork", s.authMiddleware(s.handleForkChat))

	// Workflow endpoints
	mux.HandleFunc("GET /workflows", s.authMiddleware(s.handleListWorkflows))
	mux.HandleFunc("POST /workflows", s.authMiddleware(s.handleCreateWorkflow))
	mux.HandleFunc("POST /workflows/{id}/run", s.authMiddleware(s.handleRunWorkflow))

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

// createProvider creates the LLM provider based on configuration.
func createProvider(appConfig *config.Config) (provider.Provider, error) {
	switch appConfig.Providers.Default {
	case "mock":
		cfg := mockprovider.Config{
			Mode:          appConfig.Providers.Mock.Mode,
			FixedResponse: appConfig.Providers.Mock.FixedResponse,
		}
		if appConfig.Providers.Mock.Delay != "" {
			d, err := time.ParseDuration(appConfig.Providers.Mock.Delay)
			if err != nil {
				return nil, fmt.Errorf("invalid mock delay: %w", err)
			}
			cfg.Delay = d
		}
		if appConfig.Providers.Mock.ChunkDelay != "" {
			d, err := time.ParseDuration(appConfig.Providers.Mock.ChunkDelay)
			if err != nil {
				return nil, fmt.Errorf("invalid mock chunk_delay: %w", err)
			}
			cfg.ChunkDelay = d
		}
		log.Printf("Using mock provider (mode: %s)", cfg.Mode)
		return mockprovider.New(cfg), nil
	default:
		apiKey := appConfig.Providers.Anthropic.APIKey
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
		}
		return anthropic.New(apiKey), nil
	}
}
