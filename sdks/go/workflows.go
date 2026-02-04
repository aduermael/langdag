package langdag

import (
	"context"
	"fmt"
	"net/http"
)

// ListWorkflows returns all workflow templates.
func (c *Client) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	var workflows []Workflow
	if err := c.doRequest(ctx, http.MethodGet, "/workflows", nil, &workflows); err != nil {
		return nil, err
	}
	return workflows, nil
}

// CreateWorkflow creates a new workflow template.
func (c *Client) CreateWorkflow(ctx context.Context, req *CreateWorkflowRequest) (*Workflow, error) {
	var workflow Workflow
	if err := c.doRequest(ctx, http.MethodPost, "/workflows", req, &workflow); err != nil {
		return nil, err
	}
	return &workflow, nil
}

// RunWorkflow executes a workflow template, creating a new DAG instance.
// The workflowID can be a workflow ID or name.
func (c *Client) RunWorkflow(ctx context.Context, workflowID string, req *RunWorkflowRequest) (*RunWorkflowResponse, error) {
	var resp RunWorkflowResponse
	if err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/workflows/%s/run", workflowID), req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
