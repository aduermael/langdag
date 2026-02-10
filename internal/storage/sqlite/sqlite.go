// Package sqlite provides a SQLite implementation of the storage interface.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/langdag/langdag/pkg/types"
	_ "modernc.org/sqlite"
)

// nodeColumns is the column list for node queries (unqualified).
const nodeColumns = `id, parent_id, sequence, node_type, content, model, tokens_in, tokens_out, latency_ms, status, title, system_prompt, created_at`

// nodeColumnsQ returns the column list qualified with a table alias.
func nodeColumnsQ(alias string) string {
	return alias + `.id, ` + alias + `.parent_id, ` + alias + `.sequence, ` + alias + `.node_type, ` + alias + `.content, ` + alias + `.model, ` + alias + `.tokens_in, ` + alias + `.tokens_out, ` + alias + `.latency_ms, ` + alias + `.status, ` + alias + `.title, ` + alias + `.system_prompt, ` + alias + `.created_at`
}

// SQLiteStorage implements the Storage interface using SQLite.
type SQLiteStorage struct {
	db   *sql.DB
	path string
}

// New creates a new SQLite storage instance.
func New(path string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &SQLiteStorage{
		db:   db,
		path: path,
	}, nil
}

// Init initializes the database schema.
func (s *SQLiteStorage) Init(ctx context.Context) error {
	// Check current schema version
	var version int
	err := s.db.QueryRowContext(ctx, "SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		// Table doesn't exist, run all migrations
		version = 0
	}

	// Run migrations that haven't been applied
	for i := version; i < len(migrations); i++ {
		if _, err := s.db.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("failed to run migration %d: %w", i+1, err)
		}
	}
	return nil
}

// Close closes the database connection.
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// =============================================================================
// Workflow Operations (Templates)
// =============================================================================

// CreateWorkflow creates a new workflow.
func (s *SQLiteStorage) CreateWorkflow(ctx context.Context, workflow *types.Workflow) error {
	definition, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO workflows (id, name, version, definition, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, workflow.ID, workflow.Name, workflow.Version, definition, workflow.CreatedAt, workflow.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create workflow: %w", err)
	}
	return nil
}

// GetWorkflow retrieves a workflow by ID.
func (s *SQLiteStorage) GetWorkflow(ctx context.Context, id string) (*types.Workflow, error) {
	var definition []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT definition FROM workflows WHERE id = ?
	`, id).Scan(&definition)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	var workflow types.Workflow
	if err := json.Unmarshal(definition, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}
	return &workflow, nil
}

// GetWorkflowByName retrieves a workflow by name.
func (s *SQLiteStorage) GetWorkflowByName(ctx context.Context, name string) (*types.Workflow, error) {
	var definition []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT definition FROM workflows WHERE name = ?
	`, name).Scan(&definition)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	var workflow types.Workflow
	if err := json.Unmarshal(definition, &workflow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}
	return &workflow, nil
}

// ListWorkflows returns all workflows.
func (s *SQLiteStorage) ListWorkflows(ctx context.Context) ([]*types.Workflow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT definition FROM workflows ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	defer rows.Close()

	var workflows []*types.Workflow
	for rows.Next() {
		var definition []byte
		if err := rows.Scan(&definition); err != nil {
			return nil, fmt.Errorf("failed to scan workflow: %w", err)
		}

		var workflow types.Workflow
		if err := json.Unmarshal(definition, &workflow); err != nil {
			return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
		}
		workflows = append(workflows, &workflow)
	}
	return workflows, rows.Err()
}

// UpdateWorkflow updates an existing workflow.
func (s *SQLiteStorage) UpdateWorkflow(ctx context.Context, workflow *types.Workflow) error {
	definition, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	workflow.UpdatedAt = time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE workflows SET name = ?, version = ?, definition = ?, updated_at = ?
		WHERE id = ?
	`, workflow.Name, workflow.Version, definition, workflow.UpdatedAt, workflow.ID)
	if err != nil {
		return fmt.Errorf("failed to update workflow: %w", err)
	}
	return nil
}

// DeleteWorkflow deletes a workflow.
func (s *SQLiteStorage) DeleteWorkflow(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workflows WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}
	return nil
}

// =============================================================================
// Node Operations
// =============================================================================

// scanNode scans a node from a SQL row.
func scanNode(scanner interface{ Scan(...any) error }) (*types.Node, error) {
	var node types.Node
	var parentID, model, status, title, systemPrompt sql.NullString
	var tokensIn, tokensOut, latencyMs sql.NullInt64

	err := scanner.Scan(
		&node.ID, &parentID, &node.Sequence, &node.NodeType, &node.Content,
		&model, &tokensIn, &tokensOut, &latencyMs, &status,
		&title, &systemPrompt, &node.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	node.ParentID = parentID.String
	node.Model = model.String
	node.TokensIn = int(tokensIn.Int64)
	node.TokensOut = int(tokensOut.Int64)
	node.LatencyMs = int(latencyMs.Int64)
	node.Status = status.String
	node.Title = title.String
	node.SystemPrompt = systemPrompt.String

	return &node, nil
}

// scanNodes scans multiple nodes from SQL rows.
func scanNodes(rows *sql.Rows) ([]*types.Node, error) {
	var nodes []*types.Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node: %w", err)
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// CreateNode creates a new node.
func (s *SQLiteStorage) CreateNode(ctx context.Context, node *types.Node) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nodes (`+nodeColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, node.ID, nullString(node.ParentID), node.Sequence, node.NodeType, node.Content,
		nullString(node.Model), node.TokensIn, node.TokensOut, node.LatencyMs, nullString(node.Status),
		nullString(node.Title), nullString(node.SystemPrompt), node.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}
	return nil
}

// GetNode retrieves a node by ID.
func (s *SQLiteStorage) GetNode(ctx context.Context, id string) (*types.Node, error) {
	node, err := scanNode(s.db.QueryRowContext(ctx, `
		SELECT `+nodeColumns+` FROM nodes WHERE id = ?
	`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}
	return node, nil
}

// GetNodeByPrefix retrieves a node by ID prefix.
func (s *SQLiteStorage) GetNodeByPrefix(ctx context.Context, prefix string) (*types.Node, error) {
	node, err := scanNode(s.db.QueryRowContext(ctx, `
		SELECT `+nodeColumns+` FROM nodes WHERE id LIKE ? || '%' LIMIT 1
	`, prefix))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get node by prefix: %w", err)
	}
	return node, nil
}

// GetNodeChildren retrieves direct children of a node.
func (s *SQLiteStorage) GetNodeChildren(ctx context.Context, parentID string) ([]*types.Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+nodeColumns+` FROM nodes
		WHERE parent_id = ?
		ORDER BY sequence ASC
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node children: %w", err)
	}
	defer rows.Close()
	return scanNodes(rows)
}

// GetSubtree retrieves a node and all its descendants.
func (s *SQLiteStorage) GetSubtree(ctx context.Context, nodeID string) ([]*types.Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT `+nodeColumns+` FROM nodes WHERE id = ?
			UNION ALL
			SELECT `+nodeColumnsQ("n")+` FROM nodes n
			JOIN subtree s ON n.parent_id = s.id
		)
		SELECT `+nodeColumns+` FROM subtree ORDER BY sequence ASC
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subtree: %w", err)
	}
	defer rows.Close()
	return scanNodes(rows)
}

// GetAncestors retrieves the path from root to the given node (inclusive), ordered root-first.
func (s *SQLiteStorage) GetAncestors(ctx context.Context, nodeID string) ([]*types.Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT `+nodeColumns+` FROM nodes WHERE id = ?
			UNION ALL
			SELECT `+nodeColumnsQ("n")+` FROM nodes n
			JOIN ancestors a ON n.id = a.parent_id
		)
		SELECT `+nodeColumns+` FROM ancestors ORDER BY sequence ASC
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ancestors: %w", err)
	}
	defer rows.Close()
	return scanNodes(rows)
}

// ListRootNodes returns all root nodes (nodes with no parent), ordered by creation time.
func (s *SQLiteStorage) ListRootNodes(ctx context.Context) ([]*types.Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+nodeColumns+` FROM nodes
		WHERE parent_id IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list root nodes: %w", err)
	}
	defer rows.Close()
	return scanNodes(rows)
}

// UpdateNode updates an existing node.
func (s *SQLiteStorage) UpdateNode(ctx context.Context, node *types.Node) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE nodes SET content = ?, model = ?, tokens_in = ?, tokens_out = ?,
			latency_ms = ?, status = ?, title = ?, system_prompt = ?
		WHERE id = ?
	`, node.Content, nullString(node.Model), node.TokensIn, node.TokensOut,
		node.LatencyMs, nullString(node.Status), nullString(node.Title), nullString(node.SystemPrompt),
		node.ID)
	if err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}
	return nil
}

// DeleteNode deletes a node and all its descendants.
func (s *SQLiteStorage) DeleteNode(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM nodes WHERE id = ?
			UNION ALL
			SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id
		)
		DELETE FROM nodes WHERE id IN (SELECT id FROM subtree)
	`, id)
	if err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}
	return nil
}

// nullString returns a sql.NullString from a string.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
