# SDK, CLI & Data Model Simplification

Everything is a **node**. There is no separate DAG entity. A DAG is simply the tree hanging off a root node (a node with no parent). The node ID is the only identifier needed.

## Design Philosophy

- **One concept**: nodes. A root node defines a conversation/workflow tree.
- **One verb**: `Prompt`. Same method on `Client` (new tree) and `Node` (extend tree).
- **One ID space**: node IDs. No DAG IDs. "List DAGs" = "list root nodes."
- **CLI mirrors SDK**: `langdag prompt` works like `client.Prompt()` / `node.Prompt()`.

## Data Model

### Current (being replaced)

Two tables: `dags` (metadata: title, status, model, system_prompt, tools, input, output, forked_from) and `dag_nodes` (id, dag_id, parent_id, sequence, node_type, content, model, tokens, latency, status).

### New: Everything is nodes

Single `nodes` table. Root nodes (parent_id = NULL) carry the metadata that used to live on the DAG.

```sql
CREATE TABLE nodes (
    id TEXT PRIMARY KEY,
    parent_id TEXT REFERENCES nodes(id),
    sequence INTEGER NOT NULL,
    node_type TEXT NOT NULL,           -- "user", "assistant", "system", "tool_call", "tool_result"
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

CREATE INDEX idx_nodes_parent ON nodes(parent_id);
CREATE INDEX idx_nodes_root ON nodes(parent_id) WHERE parent_id IS NULL;
```

Key changes:
- `title` lives on the root node (set from first user message or explicitly)
- `system_prompt` lives on the root node
- `status` is per-node (not per-DAG)
- `model` is per-node (different nodes can use different models)
- No `dag_id` column — a node's tree is found by traversing `parent_id` to root
- `forked_from` concept disappears — forking is just adding a child to a non-leaf node

### Migration

- For each existing DAG: its first node becomes the root, with `title` and `system_prompt` copied from the DAG row
- All `dag_nodes.dag_id` references are replaced by parent_id chains
- The `dags` table is dropped

## Server REST API

```
POST   /prompt                     Start new tree (returns assistant node)
POST   /nodes/{id}/prompt          Prompt from existing node (returns assistant node)
GET    /nodes                      List root nodes (the "DAGs")
GET    /nodes/{id}                 Get a single node
GET    /nodes/{id}/tree            Get full tree from this node
DELETE /nodes/{id}                 Delete node and its subtree
GET    /health                     Health check
```

All endpoints support `?stream=true` for SSE where applicable.

The `/prompt` and `/nodes/{id}/prompt` endpoints accept:
```json
{
    "message": "string",
    "model": "string (optional)",
    "system_prompt": "string (optional, only for /prompt)",
    "stream": false
}
```

## Go SDK

```go
client := langdag.NewClient("http://localhost:8080")

// New conversation
node, err := client.Prompt(ctx, "What is LangDAG?", langdag.WithSystem("You are helpful."))
fmt.Println(node.Content)

// Continue from any node
node2, err := node.Prompt(ctx, "Tell me more")

// Branch from earlier node
alt, err := node.Prompt(ctx, "Different angle")

// Streaming
stream, err := node.PromptStream(ctx, "Tell me more")
for event := range stream.Events() {
    fmt.Print(event.Content)
}
result, err := stream.Node()

// Get any node by ID
n, err := client.GetNode(ctx, nodeID)
reply, err := n.Prompt(ctx, "Expand on this")

// Get the full tree
tree, err := client.GetTree(ctx, rootNodeID)
for _, n := range tree.Nodes {
    fmt.Printf("[%s] %s\n", n.Type, n.Content[:50])
}

// List root nodes ("list DAGs")
roots, err := client.ListRoots(ctx)
```

### Core Types

```go
type Node struct {
    ID           string
    ParentID     string
    Type         string    // "user", "assistant", "system", etc.
    Content      string
    Sequence     int
    Model        string
    TokensIn     int
    TokensOut    int
    LatencyMs    int
    Status       string
    Title        string    // only on root nodes
    SystemPrompt string    // only on root nodes
    CreatedAt    time.Time

    client       *Client   // unexported — enables Prompt()
}

type Tree struct {
    Root  *Node
    Nodes []Node
}

type Stream struct { /* ... */ }
func (s *Stream) Events() <-chan SSEEvent
func (s *Stream) Node() (*Node, error)
```

### Client Methods

```go
func (c *Client) Prompt(ctx, message, ...PromptOption) (*Node, error)
func (c *Client) PromptStream(ctx, message, ...PromptOption) (*Stream, error)
func (c *Client) GetNode(ctx, id) (*Node, error)
func (c *Client) GetTree(ctx, id) (*Tree, error)
func (c *Client) ListRoots(ctx) ([]Node, error)
func (c *Client) DeleteNode(ctx, id) error
func (c *Client) Health(ctx) error
```

### Node Methods

```go
func (n *Node) Prompt(ctx, message) (*Node, error)
func (n *Node) PromptStream(ctx, message) (*Stream, error)
func (n *Node) Children() []Node  // when tree was fetched
```

### Prompt Options

```go
func WithSystem(prompt string) PromptOption  // only for client.Prompt (new tree)
func WithModel(model string) PromptOption
```

## CLI

```
langdag prompt "What is LangDAG?"                    # new tree
langdag prompt <node-id> "Tell me more"              # continue from node
langdag prompt                                       # interactive mode (new tree)
langdag prompt <node-id>                             # interactive mode from node

langdag ls                                           # list root nodes
langdag show <node-id>                               # show tree from node
langdag rm <node-id>                                 # delete node + subtree
```

The `langdag prompt` command replaces both `langdag chat new` and `langdag chat continue`. Interactive mode enters a REPL.

Flags:
```
langdag prompt -m <model> -s "system prompt" "message"
langdag prompt <node-id> -m <model> "message"
```

---

## Phase 1: Server Data Model

Migrate from dags + dag_nodes to a single nodes table.

- [x] 1a: Create new `nodes` table schema and migration from existing dags/dag_nodes
- [x] 1b: Update storage interface — replace DAG methods with node-only methods
- [x] 1c: Update SQLite storage implementation
- [x] 1d: Update storage tests

---

## Phase 2: Server REST API

Replace DAG-centric endpoints with node-centric endpoints.

- [x] 2a: Implement `POST /prompt` and `POST /nodes/{id}/prompt`
- [x] 2b: Implement `GET /nodes`, `GET /nodes/{id}`, `GET /nodes/{id}/tree`
- [x] 2c: Implement `DELETE /nodes/{id}`
- [x] 2d: Update API tests

---

## Phase 3: CLI

Replace `chat new` / `chat continue` with unified `prompt` command.

- [x] 3a: Implement `langdag prompt` command (new tree + continue from node + interactive mode)
- [x] 3b: Update `langdag ls` to list root nodes
- [x] 3c: Update `langdag show` to show node tree
- [x] 3d: Update `langdag rm` to delete node subtree

---

## Phase 4: Go SDK

- [x] 4a: Rewrite Go SDK with `Prompt`/`PromptStream`, `GetNode`, `GetTree`, `ListRoots`
- [x] 4b: Update Go SDK unit tests
- [x] 4c: Rewrite `examples/go/main.go`

---

**Parallel Phases: 5, 6**

## Phase 5: Python SDK

- [x] 5a: Rewrite Python SDK with `node.prompt()` / `node.prompt_stream()` pattern
- [x] 5b: Update Python SDK unit tests
- [x] 5c: Rewrite `examples/python/example.py`

---

## Phase 6: TypeScript SDK

- [x] 6a: Rewrite TypeScript SDK with `node.prompt()` / `node.promptStream()` pattern
- [x] 6b: Update TypeScript SDK unit tests
- [x] 6c: Rewrite `examples/typescript/example.ts`

---

## Phase 7: Server Cleanup

Fix remaining server-side inconsistencies.

- [ ] 7a: Fix `serve.go` startup message — still prints old `/dags`, `/chat` endpoint listing
- [x] 7b: Update API tests (was 2d)

---

## Phase 8: Docs & Website

Update all documentation to reflect the new node-centric API and unified `prompt` command.

- [ ] 8a: Update `README.md` — CLI section (prompt replaces chat new/continue), API endpoints (/nodes, /prompt), SDK examples (Prompt/PromptStream pattern)
- [ ] 8b: Update `docs/index.html` (website) — CLI demo section (replace `chat new`/`chat continue` with `prompt`), SDK code examples for all 3 languages (replace chat/fork_chat/list_dags with prompt/list_roots/get_tree)
- [ ] 8c: Update `docs/llms.txt` — concise summary with correct API/CLI/SDK info
- [ ] 8d: Update `docs/llms-full.txt` — full reference with correct API/CLI/SDK info
- [ ] 8e: Update `docs/DESIGN.md` — update or add note that schema/API sections reflect new node-centric model
- [ ] 8f: Update SDK READMEs (`sdks/go/README.md`, `sdks/python/README.md`, `sdks/typescript/README.md`)
- [ ] 8g: Update example READMEs (`examples/go/README.md`, `examples/python/README.md`, `examples/typescript/README.md`)
- [ ] 8h: Update or create OpenAPI spec for new endpoints

---

## Notes

- **Workflows**: The `workflows` table stays as-is for now (templates are a separate concern). Workflow execution will create nodes like everything else.
- **Node ID prefix matching**: Keep the current behavior where short prefixes resolve to full IDs.
- **Backward compatibility**: This is a breaking change to the API, SDK, and CLI. The migration handles existing data.
- **Streaming**: Channel-based `stream.Events()` for unidirectional. Bi-directional (interrupts) is a future concern.
