package sqlite

// migrations contains the SQL migrations for the SQLite database.
var migrations = []string{
	// Migration 1: Create initial tables
	`
	-- DAG definitions (for Workflow mode)
	CREATE TABLE IF NOT EXISTS dags (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		version INTEGER DEFAULT 1,
		definition JSON NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Conversations (for Conversation mode)
	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		title TEXT,
		model TEXT NOT NULL,
		system_prompt TEXT,
		tools JSON,
		forked_from_conv TEXT REFERENCES conversations(id),
		forked_from_node TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Nodes in a conversation (the actual DAG structure)
	CREATE TABLE IF NOT EXISTS conversation_nodes (
		id TEXT PRIMARY KEY,
		conversation_id TEXT REFERENCES conversations(id) ON DELETE CASCADE,
		parent_id TEXT REFERENCES conversation_nodes(id),
		sequence INTEGER NOT NULL,
		node_type TEXT NOT NULL,
		content JSON NOT NULL,
		model TEXT,
		tokens_in INTEGER,
		tokens_out INTEGER,
		latency_ms INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Workflow execution runs
	CREATE TABLE IF NOT EXISTS runs (
		id TEXT PRIMARY KEY,
		dag_id TEXT REFERENCES dags(id),
		status TEXT CHECK(status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
		input JSON,
		output JSON,
		state JSON,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		error TEXT
	);

	-- Individual node executions (for workflow runs)
	CREATE TABLE IF NOT EXISTS node_runs (
		id TEXT PRIMARY KEY,
		run_id TEXT REFERENCES runs(id),
		node_id TEXT NOT NULL,
		node_type TEXT NOT NULL,
		status TEXT,
		input JSON,
		output JSON,
		tokens_in INTEGER,
		tokens_out INTEGER,
		latency_ms INTEGER,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		error TEXT
	);

	-- Indexes
	CREATE INDEX IF NOT EXISTS idx_runs_dag ON runs(dag_id);
	CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
	CREATE INDEX IF NOT EXISTS idx_node_runs_run ON node_runs(run_id);
	CREATE INDEX IF NOT EXISTS idx_conv_nodes_conv ON conversation_nodes(conversation_id);
	CREATE INDEX IF NOT EXISTS idx_conv_nodes_parent ON conversation_nodes(parent_id);
	CREATE INDEX IF NOT EXISTS idx_conversations_fork ON conversations(forked_from_conv);

	-- Schema version tracking
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	);
	INSERT OR IGNORE INTO schema_version (version) VALUES (1);
	`,

	// Migration 2: Unify DAGs and Conversations
	`
	-- Step 1: Rename dags table to workflows
	ALTER TABLE dags RENAME TO workflows;

	-- Step 2: Create new unified dags table
	CREATE TABLE IF NOT EXISTS dags_new (
		id TEXT PRIMARY KEY,
		title TEXT,
		workflow_id TEXT REFERENCES workflows(id),
		model TEXT,
		system_prompt TEXT,
		tools JSON,
		status TEXT CHECK(status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
		input JSON,
		output JSON,
		forked_from_dag TEXT REFERENCES dags_new(id),
		forked_from_node TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Step 3: Create new unified dag_nodes table
	CREATE TABLE IF NOT EXISTS dag_nodes (
		id TEXT PRIMARY KEY,
		dag_id TEXT REFERENCES dags_new(id) ON DELETE CASCADE,
		parent_id TEXT REFERENCES dag_nodes(id),
		sequence INTEGER NOT NULL,
		node_type TEXT NOT NULL,
		content JSON NOT NULL,
		model TEXT,
		tokens_in INTEGER,
		tokens_out INTEGER,
		latency_ms INTEGER,
		status TEXT,
		input JSON,
		output JSON,
		error TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Step 4: Migrate conversations to dags_new
	INSERT INTO dags_new (id, title, workflow_id, model, system_prompt, tools, status, forked_from_dag, forked_from_node, created_at, updated_at)
	SELECT id, title, NULL, model, system_prompt, tools, 'completed', forked_from_conv, forked_from_node, created_at, updated_at
	FROM conversations;

	-- Step 5: Migrate runs to dags_new (with workflow_id reference)
	INSERT INTO dags_new (id, title, workflow_id, model, system_prompt, tools, status, input, output, created_at, updated_at)
	SELECT r.id, w.name, r.dag_id, NULL, NULL, NULL, r.status, r.input, r.output, r.started_at, r.completed_at
	FROM runs r
	LEFT JOIN workflows w ON r.dag_id = w.id;

	-- Step 6: Migrate conversation_nodes to dag_nodes
	INSERT INTO dag_nodes (id, dag_id, parent_id, sequence, node_type, content, model, tokens_in, tokens_out, latency_ms, status, created_at)
	SELECT id, conversation_id, parent_id, sequence, node_type, content, model, tokens_in, tokens_out, latency_ms, 'completed', created_at
	FROM conversation_nodes;

	-- Step 7: Migrate node_runs to dag_nodes
	INSERT INTO dag_nodes (id, dag_id, parent_id, sequence, node_type, content, model, tokens_in, tokens_out, latency_ms, status, input, output, error, created_at)
	SELECT id, run_id, NULL, 0, node_type, '{}', NULL, tokens_in, tokens_out, latency_ms, status, input, output, error, started_at
	FROM node_runs;

	-- Step 8: Rename dags_new to dags
	DROP TABLE IF EXISTS dags;
	ALTER TABLE dags_new RENAME TO dags;

	-- Step 9: Create indexes on new tables
	CREATE INDEX IF NOT EXISTS idx_dags_workflow ON dags(workflow_id);
	CREATE INDEX IF NOT EXISTS idx_dags_status ON dags(status);
	CREATE INDEX IF NOT EXISTS idx_dags_fork ON dags(forked_from_dag);
	CREATE INDEX IF NOT EXISTS idx_dag_nodes_dag ON dag_nodes(dag_id);
	CREATE INDEX IF NOT EXISTS idx_dag_nodes_parent ON dag_nodes(parent_id);

	-- Step 10: Drop old tables
	DROP TABLE IF EXISTS conversation_nodes;
	DROP TABLE IF EXISTS conversations;
	DROP TABLE IF EXISTS node_runs;
	DROP TABLE IF EXISTS runs;

	-- Update schema version
	UPDATE schema_version SET version = 2;
	`,
}
