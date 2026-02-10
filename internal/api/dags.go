package api

import (
	"net/http"

	"github.com/langdag/langdag/pkg/types"
)

// NodeResponse represents a node in API responses.
type NodeResponse struct {
	ID           string `json:"id"`
	ParentID     string `json:"parent_id,omitempty"`
	Sequence     int    `json:"sequence"`
	NodeType     string `json:"node_type"`
	Content      string `json:"content"`
	Model        string `json:"model,omitempty"`
	TokensIn     int    `json:"tokens_in,omitempty"`
	TokensOut    int    `json:"tokens_out,omitempty"`
	LatencyMs    int    `json:"latency_ms,omitempty"`
	Status       string `json:"status,omitempty"`
	Title        string `json:"title,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// handleListNodes returns all root nodes ("list DAGs").
func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	roots, err := s.convMgr.ListRoots(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]NodeResponse, len(roots))
	for i, n := range roots {
		response[i] = toNodeResponse(n)
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetNode returns a single node.
func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodeID := r.PathValue("id")

	node, err := s.convMgr.ResolveNode(ctx, nodeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if node == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	writeJSON(w, http.StatusOK, toNodeResponse(node))
}

// handleGetTree returns a node and its full subtree.
func (s *Server) handleGetTree(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodeID := r.PathValue("id")

	node, err := s.convMgr.ResolveNode(ctx, nodeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if node == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	nodes, err := s.convMgr.GetSubtree(ctx, node.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]NodeResponse, len(nodes))
	for i, n := range nodes {
		response[i] = toNodeResponse(n)
	}

	writeJSON(w, http.StatusOK, response)
}

// handleDeleteNode deletes a node and its subtree.
func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodeID := r.PathValue("id")

	node, err := s.convMgr.ResolveNode(ctx, nodeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if node == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	if err := s.convMgr.DeleteNode(ctx, node.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": node.ID})
}

func toNodeResponse(n *types.Node) NodeResponse {
	return NodeResponse{
		ID:           n.ID,
		ParentID:     n.ParentID,
		Sequence:     n.Sequence,
		NodeType:     string(n.NodeType),
		Content:      n.Content,
		Model:        n.Model,
		TokensIn:     n.TokensIn,
		TokensOut:    n.TokensOut,
		LatencyMs:    n.LatencyMs,
		Status:       n.Status,
		Title:        n.Title,
		SystemPrompt: n.SystemPrompt,
		CreatedAt:    n.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
