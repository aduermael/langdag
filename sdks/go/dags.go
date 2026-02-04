package langdag

import (
	"context"
	"fmt"
	"net/http"
)

// ListDAGs returns all DAG instances (conversations and workflow runs).
func (c *Client) ListDAGs(ctx context.Context) ([]DAG, error) {
	var dags []DAG
	if err := c.doRequest(ctx, http.MethodGet, "/dags", nil, &dags); err != nil {
		return nil, err
	}
	return dags, nil
}

// GetDAG returns a DAG with all its nodes.
// The id parameter can be a full UUID or a prefix (minimum 4 characters).
func (c *Client) GetDAG(ctx context.Context, id string) (*DAGDetail, error) {
	var dag DAGDetail
	if err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/dags/%s", id), nil, &dag); err != nil {
		return nil, err
	}
	return &dag, nil
}

// DeleteDAG deletes a DAG and all its nodes.
// The id parameter can be a full UUID or a prefix (minimum 4 characters).
func (c *Client) DeleteDAG(ctx context.Context, id string) (*DeleteResponse, error) {
	var resp DeleteResponse
	if err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/dags/%s", id), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
