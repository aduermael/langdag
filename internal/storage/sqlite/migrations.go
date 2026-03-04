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
}
