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
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Defaults    types.WorkflowDefaults `json:"defaults,omitempty"`
	Tools       []types.ToolDefinition `json:"tools,omitempty"`
	Nodes       []types.WorkflowNode   `json:"nodes"`
	Edges       []types.Edge           `json:"edges"`
}

// handleListWorkflows returns all workflows.
func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workflows, err := s.workflowMgr.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

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
	workflowID := r.PathValue("id")

	wf, err := s.resolveWorkflow(r.Context(), workflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if wf == nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}

	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error":       "workflow execution not yet implemented in API",
		"workflow_id": wf.ID,
		"workflow":    wf.Name,
	})
}

// resolveWorkflow finds a workflow by ID, name, or prefix.
func (s *Server) resolveWorkflow(ctx context.Context, identifier string) (*types.Workflow, error) {
	wf, err := s.workflowMgr.Get(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if wf != nil {
		return wf, nil
	}

	wf, err = s.workflowMgr.GetByName(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if wf != nil {
		return wf, nil
	}

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
