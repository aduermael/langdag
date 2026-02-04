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
// DAG Operations (Instances)
// =============================================================================

// CreateDAG creates a new DAG instance.
func (s *SQLiteStorage) CreateDAG(ctx context.Context, dag *types.DAG) error {
	var toolsJSON []byte
	var err error
	if dag.Tools != nil {
		toolsJSON, err = json.Marshal(dag.Tools)
		if err != nil {
			return fmt.Errorf("failed to marshal tools: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO dags (id, title, workflow_id, model, system_prompt, tools, status, input, output, forked_from_dag, forked_from_node, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, dag.ID, dag.Title, nullString(dag.WorkflowID), dag.Model, dag.SystemPrompt, toolsJSON, dag.Status, dag.Input, dag.Output, nullString(dag.ForkedFromDAG), nullString(dag.ForkedFromNode), dag.CreatedAt, dag.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create DAG: %w", err)
	}
	return nil
}

// GetDAG retrieves a DAG instance by ID.
func (s *SQLiteStorage) GetDAG(ctx context.Context, id string) (*types.DAG, error) {
	var dag types.DAG
	var toolsJSON sql.NullString
	var workflowID, forkedFromDAG, forkedFromNode sql.NullString
	var input, output sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, workflow_id, model, system_prompt, tools, status, input, output, forked_from_dag, forked_from_node, created_at, updated_at
		FROM dags WHERE id = ?
	`, id).Scan(&dag.ID, &dag.Title, &workflowID, &dag.Model, &dag.SystemPrompt, &toolsJSON, &dag.Status, &input, &output, &forkedFromDAG, &forkedFromNode, &dag.CreatedAt, &dag.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get DAG: %w", err)
	}

	if toolsJSON.Valid && toolsJSON.String != "" {
		if err := json.Unmarshal([]byte(toolsJSON.String), &dag.Tools); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tools: %w", err)
		}
	}
	dag.WorkflowID = workflowID.String
	dag.ForkedFromDAG = forkedFromDAG.String
	dag.ForkedFromNode = forkedFromNode.String
	if input.Valid {
		dag.Input = json.RawMessage(input.String)
	}
	if output.Valid {
		dag.Output = json.RawMessage(output.String)
	}

	return &dag, nil
}

// ListDAGs returns all DAG instances.
func (s *SQLiteStorage) ListDAGs(ctx context.Context) ([]*types.DAG, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, workflow_id, model, system_prompt, tools, status, input, output, forked_from_dag, forked_from_node, created_at, updated_at
		FROM dags ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list DAGs: %w", err)
	}
	defer rows.Close()

	var dags []*types.DAG
	for rows.Next() {
		var dag types.DAG
		var toolsJSON sql.NullString
		var workflowID, forkedFromDAG, forkedFromNode sql.NullString
		var input, output sql.NullString

		if err := rows.Scan(&dag.ID, &dag.Title, &workflowID, &dag.Model, &dag.SystemPrompt, &toolsJSON, &dag.Status, &input, &output, &forkedFromDAG, &forkedFromNode, &dag.CreatedAt, &dag.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan DAG: %w", err)
		}

		if toolsJSON.Valid && toolsJSON.String != "" {
			if err := json.Unmarshal([]byte(toolsJSON.String), &dag.Tools); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tools: %w", err)
			}
		}
		dag.WorkflowID = workflowID.String
		dag.ForkedFromDAG = forkedFromDAG.String
		dag.ForkedFromNode = forkedFromNode.String
		if input.Valid {
			dag.Input = json.RawMessage(input.String)
		}
		if output.Valid {
			dag.Output = json.RawMessage(output.String)
		}

		dags = append(dags, &dag)
	}
	return dags, rows.Err()
}

// UpdateDAG updates an existing DAG instance.
func (s *SQLiteStorage) UpdateDAG(ctx context.Context, dag *types.DAG) error {
	var toolsJSON []byte
	var err error
	if dag.Tools != nil {
		toolsJSON, err = json.Marshal(dag.Tools)
		if err != nil {
			return fmt.Errorf("failed to marshal tools: %w", err)
		}
	}

	dag.UpdatedAt = time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE dags SET title = ?, model = ?, system_prompt = ?, tools = ?, status = ?, input = ?, output = ?, updated_at = ?
		WHERE id = ?
	`, dag.Title, dag.Model, dag.SystemPrompt, toolsJSON, dag.Status, dag.Input, dag.Output, dag.UpdatedAt, dag.ID)
	if err != nil {
		return fmt.Errorf("failed to update DAG: %w", err)
	}
	return nil
}

// DeleteDAG deletes a DAG instance and its nodes.
func (s *SQLiteStorage) DeleteDAG(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete DAG: %w", err)
	}
	return nil
}

// =============================================================================
// DAG Node Operations
// =============================================================================

// AddDAGNode adds a node to a DAG.
func (s *SQLiteStorage) AddDAGNode(ctx context.Context, node *types.DAGNode) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO dag_nodes (id, dag_id, parent_id, sequence, node_type, content, model, tokens_in, tokens_out, latency_ms, status, input, output, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, node.ID, node.DAGID, nullString(node.ParentID), node.Sequence, node.NodeType, node.Content, nullString(node.Model), node.TokensIn, node.TokensOut, node.LatencyMs, node.Status, node.Input, node.Output, node.Error, node.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to add DAG node: %w", err)
	}
	return nil
}

// GetDAGNodes retrieves all nodes in a DAG.
func (s *SQLiteStorage) GetDAGNodes(ctx context.Context, dagID string) ([]*types.DAGNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, dag_id, parent_id, sequence, node_type, content, model, tokens_in, tokens_out, latency_ms, status, input, output, error, created_at
		FROM dag_nodes
		WHERE dag_id = ?
		ORDER BY sequence ASC
	`, dagID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DAG nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*types.DAGNode
	for rows.Next() {
		var node types.DAGNode
		var parentID, model, status, errStr sql.NullString
		var tokensIn, tokensOut, latencyMs sql.NullInt64
		var input, output sql.NullString

		if err := rows.Scan(&node.ID, &node.DAGID, &parentID, &node.Sequence, &node.NodeType, &node.Content, &model, &tokensIn, &tokensOut, &latencyMs, &status, &input, &output, &errStr, &node.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan DAG node: %w", err)
		}

		node.ParentID = parentID.String
		node.Model = model.String
		node.TokensIn = int(tokensIn.Int64)
		node.TokensOut = int(tokensOut.Int64)
		node.LatencyMs = int(latencyMs.Int64)
		node.Status = types.DAGStatus(status.String)
		node.Error = errStr.String
		if input.Valid {
			node.Input = json.RawMessage(input.String)
		}
		if output.Valid {
			node.Output = json.RawMessage(output.String)
		}

		nodes = append(nodes, &node)
	}
	return nodes, rows.Err()
}

// GetDAGNode retrieves a single node by ID.
func (s *SQLiteStorage) GetDAGNode(ctx context.Context, id string) (*types.DAGNode, error) {
	var node types.DAGNode
	var parentID, model, status, errStr sql.NullString
	var tokensIn, tokensOut, latencyMs sql.NullInt64
	var input, output sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, dag_id, parent_id, sequence, node_type, content, model, tokens_in, tokens_out, latency_ms, status, input, output, error, created_at
		FROM dag_nodes WHERE id = ?
	`, id).Scan(&node.ID, &node.DAGID, &parentID, &node.Sequence, &node.NodeType, &node.Content, &model, &tokensIn, &tokensOut, &latencyMs, &status, &input, &output, &errStr, &node.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get DAG node: %w", err)
	}

	node.ParentID = parentID.String
	node.Model = model.String
	node.TokensIn = int(tokensIn.Int64)
	node.TokensOut = int(tokensOut.Int64)
	node.LatencyMs = int(latencyMs.Int64)
	node.Status = types.DAGStatus(status.String)
	node.Error = errStr.String
	if input.Valid {
		node.Input = json.RawMessage(input.String)
	}
	if output.Valid {
		node.Output = json.RawMessage(output.String)
	}

	return &node, nil
}

// GetLastDAGNode retrieves the last node in a DAG.
func (s *SQLiteStorage) GetLastDAGNode(ctx context.Context, dagID string) (*types.DAGNode, error) {
	var node types.DAGNode
	var parentID, model, status, errStr sql.NullString
	var tokensIn, tokensOut, latencyMs sql.NullInt64
	var input, output sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, dag_id, parent_id, sequence, node_type, content, model, tokens_in, tokens_out, latency_ms, status, input, output, error, created_at
		FROM dag_nodes
		WHERE dag_id = ?
		ORDER BY sequence DESC
		LIMIT 1
	`, dagID).Scan(&node.ID, &node.DAGID, &parentID, &node.Sequence, &node.NodeType, &node.Content, &model, &tokensIn, &tokensOut, &latencyMs, &status, &input, &output, &errStr, &node.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get last DAG node: %w", err)
	}

	node.ParentID = parentID.String
	node.Model = model.String
	node.TokensIn = int(tokensIn.Int64)
	node.TokensOut = int(tokensOut.Int64)
	node.LatencyMs = int(latencyMs.Int64)
	node.Status = types.DAGStatus(status.String)
	node.Error = errStr.String
	if input.Valid {
		node.Input = json.RawMessage(input.String)
	}
	if output.Valid {
		node.Output = json.RawMessage(output.String)
	}

	return &node, nil
}

// UpdateDAGNode updates an existing DAG node.
func (s *SQLiteStorage) UpdateDAGNode(ctx context.Context, node *types.DAGNode) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE dag_nodes SET status = ?, output = ?, tokens_in = ?, tokens_out = ?, latency_ms = ?, error = ?
		WHERE id = ?
	`, node.Status, node.Output, node.TokensIn, node.TokensOut, node.LatencyMs, node.Error, node.ID)
	if err != nil {
		return fmt.Errorf("failed to update DAG node: %w", err)
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
