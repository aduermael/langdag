package sqlite

// migrations contains the SQL migrations for the SQLite database.
// Since this is a fresh start (no existing users), we use a single migration.
var migrations = []string{
	// Migration 1: Create tables
	`
	-- Nodes: the single unified table for all conversation tree data.
	-- Root nodes (parent_id IS NULL) carry tree-level metadata (title, system_prompt).
	CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		parent_id TEXT REFERENCES nodes(id),
		sequence INTEGER NOT NULL,
		node_type TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',

		-- LLM execution metadata (on assistant nodes)
		model TEXT,
		tokens_in INTEGER,
		tokens_out INTEGER,
		latency_ms INTEGER,
		status TEXT,

		-- Root node metadata (NULL on non-root nodes)
		title TEXT,
		system_prompt TEXT,

		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
	CREATE INDEX IF NOT EXISTS idx_nodes_root ON nodes(parent_id) WHERE parent_id IS NULL;

	-- Schema version tracking
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	);
	INSERT OR IGNORE INTO schema_version (version) VALUES (1);
	`,

	// Migration 2: Add extended token tracking columns
	`
	ALTER TABLE nodes ADD COLUMN tokens_cache_read INTEGER;
	ALTER TABLE nodes ADD COLUMN tokens_cache_creation INTEGER;
	ALTER TABLE nodes ADD COLUMN tokens_reasoning INTEGER;
	UPDATE schema_version SET version = 2;
	`,

	// Migration 3: Add node aliases
	`
	CREATE TABLE IF NOT EXISTS node_aliases (
		alias TEXT PRIMARY KEY,
		node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_aliases_node ON node_aliases(node_id);
	UPDATE schema_version SET version = 3;
	`,

	// Migration 4: Add provider column for tracking which provider served a request
	`
	ALTER TABLE nodes ADD COLUMN provider TEXT;
	UPDATE schema_version SET version = 4;
	`,

	// Migration 5: Add metadata column for arbitrary JSON metadata
	`
	ALTER TABLE nodes ADD COLUMN metadata TEXT;
	UPDATE schema_version SET version = 5;
	`,

	// Migration 6: Add root_id column for O(1) root lookup from any node
	`
	ALTER TABLE nodes ADD COLUMN root_id TEXT REFERENCES nodes(id);
	UPDATE nodes SET root_id = id WHERE parent_id IS NULL;
	UPDATE nodes SET root_id = (
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id FROM nodes WHERE id = nodes.id
			UNION ALL
			SELECT n.id, n.parent_id FROM nodes n JOIN ancestors a ON n.id = a.parent_id
		)
		SELECT id FROM ancestors WHERE parent_id IS NULL
	) WHERE root_id IS NULL;
	CREATE INDEX IF NOT EXISTS idx_nodes_root_id ON nodes(root_id);
	UPDATE schema_version SET version = 6;
	`,

	// Migration 7: Add tool ID index for O(1) orphaned tool_use detection.
	// Tracks which nodes contain tool_use and tool_result blocks, so
	// buildMessages can detect orphaned tool_use via a DB query instead
	// of parsing every message's JSON content.
	`
	CREATE TABLE IF NOT EXISTS node_tool_ids (
		node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		tool_id TEXT NOT NULL,
		role TEXT NOT NULL CHECK(role IN ('use', 'result')),
		PRIMARY KEY (node_id, tool_id, role)
	);
	CREATE INDEX IF NOT EXISTS idx_tool_ids_tool ON node_tool_ids(tool_id);
	CREATE INDEX IF NOT EXISTS idx_tool_ids_node ON node_tool_ids(node_id);

	-- Backfill: index tool_use IDs from existing assistant nodes.
	INSERT OR IGNORE INTO node_tool_ids (node_id, tool_id, role)
	SELECT n.id, json_extract(j.value, '$.id'), 'use'
	FROM nodes n, json_each(n.content) j
	WHERE n.node_type = 'assistant'
	AND json_valid(n.content)
	AND json_extract(j.value, '$.type') = 'tool_use'
	AND json_extract(j.value, '$.id') IS NOT NULL;

	-- Backfill: index tool_result IDs from existing tool_result and user nodes.
	INSERT OR IGNORE INTO node_tool_ids (node_id, tool_id, role)
	SELECT n.id, json_extract(j.value, '$.tool_use_id'), 'result'
	FROM nodes n, json_each(n.content) j
	WHERE n.node_type IN ('tool_result', 'user')
	AND json_valid(n.content)
	AND json_extract(j.value, '$.type') = 'tool_result'
	AND json_extract(j.value, '$.tool_use_id') IS NOT NULL;

	UPDATE schema_version SET version = 7;
	`,
}
