package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/langdag/langdag/pkg/types"
)

// PromptRequest represents a request to start a new tree or continue from a node.
type PromptRequest struct {
	Message      string `json:"message"`
	Model        string `json:"model,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	Stream       bool   `json:"stream,omitempty"`
}

// PromptResponse represents a prompt response.
type PromptResponse struct {
	NodeID    string `json:"node_id"`
	Content   string `json:"content"`
	TokensIn  int    `json:"tokens_in,omitempty"`
	TokensOut int    `json:"tokens_out,omitempty"`
}

// handlePrompt starts a new conversation tree.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	var req PromptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if req.Model == "" {
		req.Model = "claude-sonnet-4-20250514"
	}

	if req.Stream {
		s.streamPromptResponse(w, r, "", req.Message, req.Model, req.SystemPrompt)
		return
	}

	events, err := s.convMgr.Prompt(r.Context(), req.Message, req.Model, req.SystemPrompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	content, nodeID, err := collectEvents(events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, PromptResponse{
		NodeID:  nodeID,
		Content: content,
	})
}

// handleNodePrompt continues a conversation from an existing node.
func (s *Server) handleNodePrompt(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")

	var req PromptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Resolve node ID (support prefix matching)
	node, err := s.convMgr.ResolveNode(r.Context(), nodeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if node == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	if req.Stream {
		s.streamPromptResponse(w, r, node.ID, req.Message, req.Model, "")
		return
	}

	events, err := s.convMgr.PromptFrom(r.Context(), node.ID, req.Message, req.Model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	content, respNodeID, err := collectEvents(events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, PromptResponse{
		NodeID:  respNodeID,
		Content: content,
	})
}

// collectEvents drains an events channel and returns the collected content and node ID.
func collectEvents(events <-chan types.StreamEvent) (string, string, error) {
	var content string
	var nodeID string
	for event := range events {
		switch event.Type {
		case types.StreamEventDelta:
			content += event.Content
		case types.StreamEventError:
			return "", "", event.Error
		case types.StreamEventNodeSaved:
			nodeID = event.NodeID
		}
	}
	return content, nodeID, nil
}

// streamPromptResponse streams the response via SSE.
func (s *Server) streamPromptResponse(w http.ResponseWriter, r *http.Request, parentNodeID, message, model, systemPrompt string) {
	ctx := r.Context()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	var events <-chan types.StreamEvent
	var err error

	if parentNodeID == "" {
		events, err = s.convMgr.Prompt(ctx, message, model, systemPrompt)
	} else {
		events, err = s.convMgr.PromptFrom(ctx, parentNodeID, message, model)
	}
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	fmt.Fprintf(w, "event: start\ndata: {}\n\n")
	flusher.Flush()

	for event := range events {
		switch event.Type {
		case types.StreamEventDelta:
			data, _ := json.Marshal(map[string]string{"content": event.Content})
			fmt.Fprintf(w, "event: delta\ndata: %s\n\n", data)
			flusher.Flush()

		case types.StreamEventNodeSaved:
			data, _ := json.Marshal(map[string]string{"node_id": event.NodeID})
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
			flusher.Flush()

		case types.StreamEventError:
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", event.Error.Error())
			flusher.Flush()
		}
	}
}
