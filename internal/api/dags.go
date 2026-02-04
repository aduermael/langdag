package api

import (
	"net/http"
	"strings"
)

// DAGResponse represents a DAG in API responses.
type DAGResponse struct {
	ID           string `json:"id"`
	Title        string `json:"title,omitempty"`
	WorkflowID   string `json:"workflow_id,omitempty"`
	Model        string `json:"model,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	NodeCount    int    `json:"node_count,omitempty"`
}

// DAGDetailResponse includes nodes.
type DAGDetailResponse struct {
	DAGResponse
	Nodes []NodeResponse `json:"nodes"`
}

// NodeResponse represents a node in API responses.
type NodeResponse struct {
	ID        string `json:"id"`
	ParentID  string `json:"parent_id,omitempty"`
	Sequence  int    `json:"sequence"`
	NodeType  string `json:"node_type"`
	Content   string `json:"content"`
	Model     string `json:"model,omitempty"`
	TokensIn  int    `json:"tokens_in,omitempty"`
	TokensOut int    `json:"tokens_out,omitempty"`
	LatencyMs int    `json:"latency_ms,omitempty"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at"`
}

// handleListDAGs returns all DAGs.
func (s *Server) handleListDAGs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	dags, err := s.convMgr.ListDAGs(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response format
	response := make([]DAGResponse, len(dags))
	for i, dag := range dags {
		response[i] = DAGResponse{
			ID:           dag.ID,
			Title:        dag.Title,
			WorkflowID:   dag.WorkflowID,
			Model:        dag.Model,
			SystemPrompt: dag.SystemPrompt,
			Status:       string(dag.Status),
			CreatedAt:    dag.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    dag.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetDAG returns a single DAG with its nodes.
func (s *Server) handleGetDAG(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dagID := r.PathValue("id")

	// Try exact match first, then prefix
	dag, err := s.convMgr.GetDAG(ctx, dagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dag == nil {
		// Try prefix match
		dags, err := s.convMgr.ListDAGs(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, d := range dags {
			if strings.HasPrefix(d.ID, dagID) {
				dag = d
				break
			}
		}
	}
	if dag == nil {
		writeError(w, http.StatusNotFound, "DAG not found")
		return
	}

	// Get nodes
	nodes, err := s.convMgr.GetNodes(ctx, dag.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response format
	nodeResponses := make([]NodeResponse, len(nodes))
	for i, node := range nodes {
		// Parse content - try to extract string
		content := string(node.Content)
		if len(content) > 2 && content[0] == '"' {
			// JSON string, unquote it
			content = content[1 : len(content)-1]
		}

		nodeResponses[i] = NodeResponse{
			ID:        node.ID,
			ParentID:  node.ParentID,
			Sequence:  node.Sequence,
			NodeType:  string(node.NodeType),
			Content:   content,
			Model:     node.Model,
			TokensIn:  node.TokensIn,
			TokensOut: node.TokensOut,
			LatencyMs: node.LatencyMs,
			Status:    string(node.Status),
			CreatedAt: node.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	response := DAGDetailResponse{
		DAGResponse: DAGResponse{
			ID:           dag.ID,
			Title:        dag.Title,
			WorkflowID:   dag.WorkflowID,
			Model:        dag.Model,
			SystemPrompt: dag.SystemPrompt,
			Status:       string(dag.Status),
			CreatedAt:    dag.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    dag.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			NodeCount:    len(nodes),
		},
		Nodes: nodeResponses,
	}

	writeJSON(w, http.StatusOK, response)
}

// handleDeleteDAG deletes a DAG.
func (s *Server) handleDeleteDAG(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dagID := r.PathValue("id")

	// Try exact match first, then prefix
	dag, err := s.convMgr.GetDAG(ctx, dagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dag == nil {
		// Try prefix match
		dags, err := s.convMgr.ListDAGs(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, d := range dags {
			if strings.HasPrefix(d.ID, dagID) {
				dag = d
				break
			}
		}
	}
	if dag == nil {
		writeError(w, http.StatusNotFound, "DAG not found")
		return
	}

	if err := s.convMgr.DeleteDAG(ctx, dag.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": dag.ID})
}
