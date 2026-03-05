package api

import (
	"net/http"

	"langdag.com/langdag/types"
)

// NodeResponse represents a node in API responses.
type NodeResponse struct {
	ID           string `json:"id"`
	ParentID     string `json:"parent_id,omitempty"`
	RootID       string `json:"root_id,omitempty"`
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

// handleGetTree returns the full conversation tree containing the given node.
// Uses root_id for O(1) root lookup, then returns the complete subtree.
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

	// Use root_id for O(1) root lookup
	rootID := node.RootID
	if rootID == "" {
		rootID = node.ID
	}

	nodes, err := s.convMgr.GetSubtree(ctx, rootID)
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

// handleCreateAlias creates an alias for a node.
func (s *Server) handleCreateAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodeID := r.PathValue("id")
	alias := r.PathValue("alias")

	node, err := s.convMgr.ResolveNode(ctx, nodeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if node == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	if err := s.convMgr.CreateAlias(ctx, node.ID, alias); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"alias": alias, "node_id": node.ID})
}

// handleListAliases lists aliases for a node.
func (s *Server) handleListAliases(w http.ResponseWriter, r *http.Request) {
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

	aliases, err := s.convMgr.ListAliases(ctx, node.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if aliases == nil {
		aliases = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"node_id": node.ID, "aliases": aliases})
}

// handleDeleteAlias deletes an alias.
func (s *Server) handleDeleteAlias(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")

	if err := s.convMgr.DeleteAlias(r.Context(), alias); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func toNodeResponse(n *types.Node) NodeResponse {
	return NodeResponse{
		ID:           n.ID,
		ParentID:     n.ParentID,
		RootID:       n.RootID,
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
