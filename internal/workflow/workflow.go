// Package workflow provides workflow template management.
package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/langdag/langdag/internal/storage"
	"github.com/langdag/langdag/pkg/types"
)

// Manager handles workflow operations.
type Manager struct {
	storage storage.Storage
}

// NewManager creates a new workflow manager.
func NewManager(store storage.Storage) *Manager {
	return &Manager{storage: store}
}

// Create creates a new workflow from a definition.
func (m *Manager) Create(ctx context.Context, workflow *types.Workflow) error {
	now := time.Now()
	workflow.ID = uuid.New().String()
	workflow.CreatedAt = now
	workflow.UpdatedAt = now
	if workflow.Version == 0 {
		workflow.Version = 1
	}

	return m.storage.CreateWorkflow(ctx, workflow)
}

// Get retrieves a workflow by ID.
func (m *Manager) Get(ctx context.Context, id string) (*types.Workflow, error) {
	return m.storage.GetWorkflow(ctx, id)
}

// GetByName retrieves a workflow by name.
func (m *Manager) GetByName(ctx context.Context, name string) (*types.Workflow, error) {
	return m.storage.GetWorkflowByName(ctx, name)
}

// List returns all workflows.
func (m *Manager) List(ctx context.Context) ([]*types.Workflow, error) {
	return m.storage.ListWorkflows(ctx)
}

// Update updates an existing workflow.
func (m *Manager) Update(ctx context.Context, workflow *types.Workflow) error {
	workflow.Version++
	return m.storage.UpdateWorkflow(ctx, workflow)
}

// Delete deletes a workflow.
func (m *Manager) Delete(ctx context.Context, id string) error {
	return m.storage.DeleteWorkflow(ctx, id)
}

// GetNode returns a node by ID from a workflow.
func GetNode(workflow *types.Workflow, nodeID string) *types.Node {
	for i := range workflow.Nodes {
		if workflow.Nodes[i].ID == nodeID {
			return &workflow.Nodes[i]
		}
	}
	return nil
}

// GetInputNode returns the input node of a workflow.
func GetInputNode(workflow *types.Workflow) *types.Node {
	for i := range workflow.Nodes {
		if workflow.Nodes[i].Type == types.NodeTypeInput {
			return &workflow.Nodes[i]
		}
	}
	return nil
}

// GetOutputNode returns the output node of a workflow.
func GetOutputNode(workflow *types.Workflow) *types.Node {
	for i := range workflow.Nodes {
		if workflow.Nodes[i].Type == types.NodeTypeOutput {
			return &workflow.Nodes[i]
		}
	}
	return nil
}

// GetChildNodes returns nodes that are children of the given node.
func GetChildNodes(workflow *types.Workflow, nodeID string) []*types.Node {
	var children []*types.Node
	for _, edge := range workflow.Edges {
		if edge.From == nodeID {
			if node := GetNode(workflow, edge.To); node != nil {
				children = append(children, node)
			}
		}
	}
	return children
}

// GetParentNodes returns nodes that are parents of the given node.
func GetParentNodes(workflow *types.Workflow, nodeID string) []*types.Node {
	var parents []*types.Node
	for _, edge := range workflow.Edges {
		if edge.To == nodeID {
			if node := GetNode(workflow, edge.From); node != nil {
				parents = append(parents, node)
			}
		}
	}
	return parents
}

// TopologicalSort returns nodes in topological order.
func TopologicalSort(workflow *types.Workflow) ([]string, error) {
	// Build adjacency list and in-degree count
	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for _, node := range workflow.Nodes {
		inDegree[node.ID] = 0
		adj[node.ID] = []string{}
	}

	for _, edge := range workflow.Edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}

	// Find nodes with no incoming edges
	var queue []string
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	var result []string
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		result = append(result, nodeID)

		for _, child := range adj[nodeID] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if len(result) != len(workflow.Nodes) {
		return nil, fmt.Errorf("workflow contains a cycle")
	}

	return result, nil
}
