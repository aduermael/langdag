package sqlite

// migrations contains the SQL migrations for the SQLite database.
// Since this is a fresh start (no existing users), we use a single migration.
var migrations = []string{
	// Migration 1: Create tables — everything is nodes
	`
	-- Workflow templates
	CREATE TABLE IF NOT EXISTS workflows (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		version INTEGER DEFAULT 1,
		definition JSON NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Nodes: the single unified table for all conversation/workflow tree data.
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
}
