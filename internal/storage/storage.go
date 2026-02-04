// Package storage defines the storage interface for langdag.
package storage

import (
	"context"

	"github.com/langdag/langdag/pkg/types"
)

// Storage defines the interface for persisting workflows, DAGs, and nodes.
type Storage interface {
	// Initialize the storage (run migrations, etc.)
	Init(ctx context.Context) error

	// Close the storage connection
	Close() error

	// Workflow operations (templates)
	CreateWorkflow(ctx context.Context, workflow *types.Workflow) error
	GetWorkflow(ctx context.Context, id string) (*types.Workflow, error)
	GetWorkflowByName(ctx context.Context, name string) (*types.Workflow, error)
	ListWorkflows(ctx context.Context) ([]*types.Workflow, error)
	UpdateWorkflow(ctx context.Context, workflow *types.Workflow) error
	DeleteWorkflow(ctx context.Context, id string) error

	// DAG operations (instances - unified from conversations and runs)
	CreateDAG(ctx context.Context, dag *types.DAG) error
	GetDAG(ctx context.Context, id string) (*types.DAG, error)
	ListDAGs(ctx context.Context) ([]*types.DAG, error)
	UpdateDAG(ctx context.Context, dag *types.DAG) error
	DeleteDAG(ctx context.Context, id string) error

	// DAG node operations (unified from conversation_nodes and node_runs)
	AddDAGNode(ctx context.Context, node *types.DAGNode) error
	GetDAGNodes(ctx context.Context, dagID string) ([]*types.DAGNode, error)
	GetDAGNode(ctx context.Context, id string) (*types.DAGNode, error)
	GetLastDAGNode(ctx context.Context, dagID string) (*types.DAGNode, error)
	UpdateDAGNode(ctx context.Context, node *types.DAGNode) error
}
