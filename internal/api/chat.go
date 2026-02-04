package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/langdag/langdag/pkg/types"
)

// NewChatRequest represents a request to start a new chat.
type NewChatRequest struct {
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	Message      string `json:"message"`
	Stream       bool   `json:"stream,omitempty"`
}

// ContinueChatRequest represents a request to continue a chat.
type ContinueChatRequest struct {
	Message string `json:"message"`
	Stream  bool   `json:"stream,omitempty"`
}

// ForkChatRequest represents a request to fork from a specific node.
type ForkChatRequest struct {
	NodeID  string `json:"node_id"`
	Message string `json:"message"`
	Stream  bool   `json:"stream,omitempty"`
}

// ChatResponse represents a chat response.
type ChatResponse struct {
	DAGID    string `json:"dag_id"`
	NodeID   string `json:"node_id"`
	Content  string `json:"content"`
	TokensIn int    `json:"tokens_in,omitempty"`
	TokensOut int   `json:"tokens_out,omitempty"`
}

// handleNewChat starts a new conversation.
func (s *Server) handleNewChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req NewChatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Default model
	if req.Model == "" {
		req.Model = "claude-sonnet-4-20250514"
	}

	// Create new DAG
	dag, err := s.convMgr.CreateDAG(ctx, req.Model, req.SystemPrompt, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Set title from first message
	title := s.convMgr.GenerateTitle(req.Message)
	s.convMgr.UpdateTitle(ctx, dag.ID, title)

	// Send message
	if req.Stream {
		s.streamChatResponse(w, r, dag.ID, "", req.Message)
		return
	}

	// Non-streaming response
	response, nodeID, err := s.sendChatMessage(ctx, dag.ID, "", req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		DAGID:     dag.ID,
		NodeID:    nodeID,
		Content:   response,
	})
}

// handleContinueChat continues an existing conversation.
func (s *Server) handleContinueChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dagID := r.PathValue("id")

	var req ContinueChatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Resolve DAG ID (support prefix matching)
	dag, err := s.resolveDAG(ctx, dagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dag == nil {
		writeError(w, http.StatusNotFound, "DAG not found")
		return
	}

	// Send message
	if req.Stream {
		s.streamChatResponse(w, r, dag.ID, "", req.Message)
		return
	}

	response, nodeID, err := s.sendChatMessage(ctx, dag.ID, "", req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		DAGID:    dag.ID,
		NodeID:   nodeID,
		Content:  response,
	})
}

// handleForkChat forks from a specific node.
func (s *Server) handleForkChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dagID := r.PathValue("id")

	var req ForkChatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NodeID == "" {
		writeError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Resolve DAG ID
	dag, err := s.resolveDAG(ctx, dagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dag == nil {
		writeError(w, http.StatusNotFound, "DAG not found")
		return
	}

	// Resolve node ID (support prefix matching)
	nodes, err := s.convMgr.GetNodes(ctx, dag.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var nodeID string
	for _, n := range nodes {
		if n.ID == req.NodeID || strings.HasPrefix(n.ID, req.NodeID) {
			nodeID = n.ID
			break
		}
	}
	if nodeID == "" {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	// Send message from specific node
	if req.Stream {
		s.streamChatResponse(w, r, dag.ID, nodeID, req.Message)
		return
	}

	response, respNodeID, err := s.sendChatMessage(ctx, dag.ID, nodeID, req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		DAGID:    dag.ID,
		NodeID:   respNodeID,
		Content:  response,
	})
}

// sendChatMessage sends a message and waits for the complete response.
func (s *Server) sendChatMessage(ctx context.Context, dagID, parentNodeID, message string) (string, string, error) {
	events, err := s.convMgr.SendMessageAfter(ctx, dagID, parentNodeID, message)
	if err != nil {
		return "", "", err
	}

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

// streamChatResponse streams the response via SSE.
func (s *Server) streamChatResponse(w http.ResponseWriter, r *http.Request, dagID, parentNodeID, message string) {
	ctx := r.Context()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	events, err := s.convMgr.SendMessageAfter(ctx, dagID, parentNodeID, message)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send initial event
	fmt.Fprintf(w, "event: start\ndata: {\"dag_id\":\"%s\"}\n\n", dagID)
	flusher.Flush()

	for event := range events {
		switch event.Type {
		case types.StreamEventDelta:
			data, _ := json.Marshal(map[string]string{"content": event.Content})
			fmt.Fprintf(w, "event: delta\ndata: %s\n\n", data)
			flusher.Flush()

		case types.StreamEventNodeSaved:
			data, _ := json.Marshal(map[string]string{"node_id": event.NodeID, "dag_id": dagID})
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
			flusher.Flush()

		case types.StreamEventError:
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", event.Error.Error())
			flusher.Flush()
		}
	}
}

// resolveDAG finds a DAG by exact ID or prefix match.
func (s *Server) resolveDAG(ctx context.Context, dagID string) (*types.DAG, error) {
	dag, err := s.convMgr.GetDAG(ctx, dagID)
	if err != nil {
		return nil, err
	}
	if dag != nil {
		return dag, nil
	}

	// Try prefix match
	dags, err := s.convMgr.ListDAGs(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range dags {
		if strings.HasPrefix(d.ID, dagID) {
			return d, nil
		}
	}

	return nil, nil
}
