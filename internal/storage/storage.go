// Package storage defines the storage interface for langdag.
package storage

import (
	"context"

	"github.com/langdag/langdag/pkg/types"
)

// Storage defines the interface for persisting workflows and nodes.
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

	// Node operations
	CreateNode(ctx context.Context, node *types.Node) error
	GetNode(ctx context.Context, id string) (*types.Node, error)
	GetNodeByPrefix(ctx context.Context, prefix string) (*types.Node, error)
	GetNodeChildren(ctx context.Context, parentID string) ([]*types.Node, error)
	GetSubtree(ctx context.Context, nodeID string) ([]*types.Node, error)
	GetAncestors(ctx context.Context, nodeID string) ([]*types.Node, error)
	ListRootNodes(ctx context.Context) ([]*types.Node, error)
	UpdateNode(ctx context.Context, node *types.Node) error
	DeleteNode(ctx context.Context, id string) error
}
