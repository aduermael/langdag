package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Server is the mock LLM HTTP server.
type Server struct {
	cfg        *Config
	httpServer *http.Server
	responder  *Responder
}

// NewServer creates a new mock LLM server.
func NewServer(cfg *Config) *Server {
	s := &Server{
		cfg:       cfg,
		responder: NewResponder(cfg),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", s.handleMessages)
	mux.HandleFunc("GET /health", s.handleHealth)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Disable for SSE
		IdleTimeout:  120 * time.Second,
	}

	return s
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleMessages handles the /v1/messages endpoint (Anthropic API compatible).
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	var req MessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "failed to decode request body")
		return
	}

	// Apply configured delay
	if s.cfg.Delay > 0 {
		time.Sleep(s.cfg.Delay)
	}

	// Error mode returns an error response
	if s.cfg.Mode == "error" {
		errType := "api_error"
		if s.cfg.ErrorCode == 429 {
			errType = "rate_limit_error"
		} else if s.cfg.ErrorCode == 401 {
			errType = "authentication_error"
		} else if s.cfg.ErrorCode >= 400 && s.cfg.ErrorCode < 500 {
			errType = "invalid_request_error"
		}
		writeJSONError(w, s.cfg.ErrorCode, errType, s.cfg.ErrorMessage)
		return
	}

	// Check if streaming is requested
	if req.Stream {
		s.handleStreamingMessages(w, r, &req)
		return
	}

	s.handleNonStreamingMessages(w, r, &req)
}

// handleNonStreamingMessages handles non-streaming message requests.
func (s *Server) handleNonStreamingMessages(w http.ResponseWriter, r *http.Request, req *MessagesRequest) {
	text := s.responder.GenerateResponse(req)

	resp := MessagesResponse{
		ID:    generateMessageID(),
		Type:  "message",
		Role:  "assistant",
		Model: req.Model,
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
		StopReason: "end_turn",
		Usage: Usage{
			InputTokens:  countInputTokens(req),
			OutputTokens: countWords(text),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleStreamingMessages handles streaming message requests via SSE.
func (s *Server) handleStreamingMessages(w http.ResponseWriter, r *http.Request, req *MessagesRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	text := s.responder.GenerateResponse(req)
	chunks := chunkText(text, s.cfg.ChunkSize)
	msgID := generateMessageID()
	inputTokens := countInputTokens(req)

	// message_start
	writeSSE(w, flusher, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":    msgID,
			"type":  "message",
			"role":  "assistant",
			"model": req.Model,
			"content": []interface{}{},
			"usage": map[string]int{
				"input_tokens":  inputTokens,
				"output_tokens": 0,
			},
		},
	})

	// content_block_start
	writeSSE(w, flusher, "content_block_start", map[string]interface{}{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]string{"type": "text", "text": ""},
	})

	// content_block_delta events (token by token)
	outputTokens := 0
	for _, chunk := range chunks {
		if r.Context().Err() != nil {
			return
		}

		writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]string{
				"type": "text_delta",
				"text": chunk,
			},
		})
		outputTokens += countWords(chunk)

		if s.cfg.ChunkDelay > 0 {
			time.Sleep(s.cfg.ChunkDelay)
		}
	}

	// content_block_stop
	writeSSE(w, flusher, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})

	// message_delta
	writeSSE(w, flusher, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]string{
			"stop_reason": "end_turn",
		},
		"usage": map[string]int{
			"output_tokens": outputTokens,
		},
	})

	// message_stop
	writeSSE(w, flusher, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

// writeSSE writes a server-sent event.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	jsonBytes, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonBytes))
	flusher.Flush()
}

// writeJSONError writes an Anthropic-style JSON error response.
func writeJSONError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"type":    errType,
			"message": message,
		},
	})
}
