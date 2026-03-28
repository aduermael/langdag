// Package sqlite provides a SQLite implementation of the storage interface.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"langdag.com/langdag/types"
	_ "modernc.org/sqlite"
)

// nodeColumns is the column list for node queries (unqualified).
const nodeColumns = `id, parent_id, root_id, sequence, node_type, content, provider, model, tokens_in, tokens_out, tokens_cache_read, tokens_cache_creation, tokens_reasoning, latency_ms, stop_reason, output_group_id, status, title, system_prompt, created_at, metadata`

// nodeColumnsQ returns the column list qualified with a table alias.
func nodeColumnsQ(alias string) string {
	return alias + `.id, ` + alias + `.parent_id, ` + alias + `.root_id, ` + alias + `.sequence, ` + alias + `.node_type, ` + alias + `.content, ` + alias + `.provider, ` + alias + `.model, ` + alias + `.tokens_in, ` + alias + `.tokens_out, ` + alias + `.tokens_cache_read, ` + alias + `.tokens_cache_creation, ` + alias + `.tokens_reasoning, ` + alias + `.latency_ms, ` + alias + `.stop_reason, ` + alias + `.output_group_id, ` + alias + `.status, ` + alias + `.title, ` + alias + `.system_prompt, ` + alias + `.created_at, ` + alias + `.metadata`
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
// Node Operations
// =============================================================================

// scanNode scans a node from a SQL row.
func scanNode(scanner interface{ Scan(...any) error }) (*types.Node, error) {
	var node types.Node
	var parentID, rootID, providerName, model, stopReason, outputGroupID, status, title, systemPrompt, metadata sql.NullString
	var tokensIn, tokensOut, tokensCacheRead, tokensCacheCreation, tokensReasoning, latencyMs sql.NullInt64

	err := scanner.Scan(
		&node.ID, &parentID, &rootID, &node.Sequence, &node.NodeType, &node.Content,
		&providerName, &model, &tokensIn, &tokensOut, &tokensCacheRead, &tokensCacheCreation, &tokensReasoning,
		&latencyMs, &stopReason, &outputGroupID, &status,
		&title, &systemPrompt, &node.CreatedAt, &metadata,
	)
	if err != nil {
		return nil, err
	}

	node.ParentID = parentID.String
	node.RootID = rootID.String
	node.Provider = providerName.String
	node.Model = model.String
	node.TokensIn = int(tokensIn.Int64)
	node.TokensOut = int(tokensOut.Int64)
	node.TokensCacheRead = int(tokensCacheRead.Int64)
	node.TokensCacheCreation = int(tokensCacheCreation.Int64)
	node.TokensReasoning = int(tokensReasoning.Int64)
	node.LatencyMs = int(latencyMs.Int64)
	node.StopReason = stopReason.String
	node.OutputGroupID = outputGroupID.String
	node.Status = status.String
	node.Title = title.String
	node.SystemPrompt = systemPrompt.String
	if metadata.Valid && metadata.String != "" {
		node.Metadata = json.RawMessage(metadata.String)
	}

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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, node.ID, nullString(node.ParentID), nullString(node.RootID), node.Sequence, node.NodeType, node.Content,
		nullString(node.Provider), nullString(node.Model), node.TokensIn, node.TokensOut, node.TokensCacheRead, node.TokensCacheCreation, node.TokensReasoning,
		node.LatencyMs, nullString(node.StopReason), nullString(node.OutputGroupID), nullString(node.Status),
		nullString(node.Title), nullString(node.SystemPrompt), node.CreatedAt, nullRawMessage(node.Metadata))
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
		UPDATE nodes SET content = ?, provider = ?, model = ?, tokens_in = ?, tokens_out = ?,
			tokens_cache_read = ?, tokens_cache_creation = ?, tokens_reasoning = ?,
			latency_ms = ?, status = ?, title = ?, system_prompt = ?, metadata = ?
		WHERE id = ?
	`, node.Content, nullString(node.Provider), nullString(node.Model), node.TokensIn, node.TokensOut,
		node.TokensCacheRead, node.TokensCacheCreation, node.TokensReasoning,
		node.LatencyMs, nullString(node.Status), nullString(node.Title), nullString(node.SystemPrompt),
		nullRawMessage(node.Metadata), node.ID)
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

// =============================================================================
// Alias Operations
// =============================================================================

// CreateAlias creates an alias for a node.
func (s *SQLiteStorage) CreateAlias(ctx context.Context, nodeID, alias string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO node_aliases (alias, node_id) VALUES (?, ?)
	`, alias, nodeID)
	if err != nil {
		return fmt.Errorf("failed to create alias: %w", err)
	}
	return nil
}

// DeleteAlias removes an alias.
func (s *SQLiteStorage) DeleteAlias(ctx context.Context, alias string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM node_aliases WHERE alias = ?`, alias)
	if err != nil {
		return fmt.Errorf("failed to delete alias: %w", err)
	}
	return nil
}

// GetNodeByAlias retrieves a node by its alias.
func (s *SQLiteStorage) GetNodeByAlias(ctx context.Context, alias string) (*types.Node, error) {
	node, err := scanNode(s.db.QueryRowContext(ctx, `
		SELECT `+nodeColumnsQ("n")+` FROM nodes n
		JOIN node_aliases a ON n.id = a.node_id
		WHERE a.alias = ?
	`, alias))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get node by alias: %w", err)
	}
	return node, nil
}

// ListAliases returns all aliases for a node.
func (s *SQLiteStorage) ListAliases(ctx context.Context, nodeID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT alias FROM node_aliases WHERE node_id = ? ORDER BY alias
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to list aliases: %w", err)
	}
	defer rows.Close()

	var aliases []string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("failed to scan alias: %w", err)
		}
		aliases = append(aliases, alias)
	}
	return aliases, rows.Err()
}

// =============================================================================
// Tool ID Index Operations
// =============================================================================

// IndexToolIDs saves tool_use or tool_result IDs for a node.
// role must be "use" or "result".
func (s *SQLiteStorage) IndexToolIDs(ctx context.Context, nodeID string, toolIDs []string, role string) error {
	if len(toolIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO node_tool_ids (node_id, tool_id, role) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert: %w", err)
	}
	defer stmt.Close()
	for _, id := range toolIDs {
		if _, err := stmt.ExecContext(ctx, nodeID, id, role); err != nil {
			return fmt.Errorf("failed to index tool ID %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// GetOrphanedToolUses returns tool_use IDs among the given ancestor node IDs
// that have no matching tool_result in the same ancestor path.
// Returns map[node_id][]orphaned_tool_use_id.
func (s *SQLiteStorage) GetOrphanedToolUses(ctx context.Context, ancestorIDs []string) (map[string][]string, error) {
	if len(ancestorIDs) == 0 {
		return nil, nil
	}

	// Build placeholders for the IN clause.
	placeholders := make([]string, len(ancestorIDs))
	args := make([]interface{}, 0, len(ancestorIDs)*2)
	for i, id := range ancestorIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ",")

	// Duplicate args for the subquery.
	for _, id := range ancestorIDs {
		args = append(args, id)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT nti.node_id, nti.tool_id
		FROM node_tool_ids nti
		WHERE nti.node_id IN (`+inClause+`) AND nti.role = 'use'
		AND nti.tool_id NOT IN (
			SELECT tool_id FROM node_tool_ids
			WHERE node_id IN (`+inClause+`) AND role = 'result'
		)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query orphaned tool uses: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var nodeID, toolID string
		if err := rows.Scan(&nodeID, &toolID); err != nil {
			return nil, fmt.Errorf("failed to scan orphaned tool use: %w", err)
		}
		result[nodeID] = append(result[nodeID], toolID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// nullString returns a sql.NullString from a string.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullRawMessage returns a sql.NullString from a json.RawMessage.
func nullRawMessage(m json.RawMessage) sql.NullString {
	if len(m) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(m), Valid: true}
}
