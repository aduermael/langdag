package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/langdag/langdag/internal/workflow"
	"github.com/langdag/langdag/pkg/types"
)

// WorkflowResponse represents a workflow in API responses.
type WorkflowResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CreateWorkflowRequest represents a request to create a workflow.
type CreateWorkflowRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Defaults    types.WorkflowDefaults  `json:"defaults,omitempty"`
	Tools       []types.ToolDefinition  `json:"tools,omitempty"`
	Nodes       []types.Node            `json:"nodes"`
	Edges       []types.Edge            `json:"edges"`
}

// RunWorkflowRequest represents a request to run a workflow.
type RunWorkflowRequest struct {
	Input  map[string]interface{} `json:"input,omitempty"`
	Stream bool                   `json:"stream,omitempty"`
}

// RunWorkflowResponse represents a workflow run response.
type RunWorkflowResponse struct {
	DAGID  string                 `json:"dag_id"`
	Status string                 `json:"status"`
	Output map[string]interface{} `json:"output,omitempty"`
}

// handleListWorkflows returns all workflows.
func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workflows, err := s.workflowMgr.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response format
	response := make([]WorkflowResponse, len(workflows))
	for i, wf := range workflows {
		response[i] = WorkflowResponse{
			ID:          wf.ID,
			Name:        wf.Name,
			Version:     wf.Version,
			Description: wf.Description,
			CreatedAt:   wf.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   wf.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleCreateWorkflow creates a new workflow.
func (s *Server) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateWorkflowRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Nodes) == 0 {
		writeError(w, http.StatusBadRequest, "at least one node is required")
		return
	}

	// Validate workflow
	wf := &types.Workflow{
		Name:        req.Name,
		Description: req.Description,
		Defaults:    req.Defaults,
		Tools:       req.Tools,
		Nodes:       req.Nodes,
		Edges:       req.Edges,
	}

	result := workflow.Validate(wf)
	if !result.Valid {
		writeError(w, http.StatusBadRequest, result.FormatErrors())
		return
	}

	// Create workflow
	if err := s.workflowMgr.Create(ctx, wf); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, WorkflowResponse{
		ID:          wf.ID,
		Name:        wf.Name,
		Version:     wf.Version,
		Description: wf.Description,
		CreatedAt:   wf.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   wf.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// handleRunWorkflow runs a workflow.
func (s *Server) handleRunWorkflow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workflowID := r.PathValue("id")

	var req RunWorkflowRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Resolve workflow (by ID or name)
	wf, err := s.resolveWorkflow(ctx, workflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if wf == nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}

	// For now, return a placeholder - full workflow execution will be implemented
	// when the executor is integrated with the API
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error":       "workflow execution not yet implemented in API",
		"workflow_id": wf.ID,
		"workflow":    wf.Name,
	})
}

// resolveWorkflow finds a workflow by ID, name, or prefix.
func (s *Server) resolveWorkflow(ctx context.Context, identifier string) (*types.Workflow, error) {
	// Try by ID first
	wf, err := s.workflowMgr.Get(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if wf != nil {
		return wf, nil
	}

	// Try by name
	wf, err = s.workflowMgr.GetByName(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if wf != nil {
		return wf, nil
	}

	// Try prefix match on ID
	workflows, err := s.workflowMgr.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, w := range workflows {
		if strings.HasPrefix(w.ID, identifier) {
			return w, nil
		}
	}

	return nil, nil
}
