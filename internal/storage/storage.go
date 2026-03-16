// Package storage defines the storage interface for langdag.
package storage

import (
	"context"

	"langdag.com/langdag/types"
)

// Storage defines the interface for persisting nodes.
type Storage interface {
	// Initialize the storage (run migrations, etc.)
	Init(ctx context.Context) error

	// Close the storage connection
	Close() error

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

	// Alias operations
	CreateAlias(ctx context.Context, nodeID, alias string) error
	DeleteAlias(ctx context.Context, alias string) error
	GetNodeByAlias(ctx context.Context, alias string) (*types.Node, error)
	ListAliases(ctx context.Context, nodeID string) ([]string, error)

	// Tool ID index operations
	IndexToolIDs(ctx context.Context, nodeID string, toolIDs []string, role string) error
	GetOrphanedToolUses(ctx context.Context, ancestorIDs []string) (map[string][]string, error)
}
