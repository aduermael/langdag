# LangDAG - LLM Conversation DAG Manager

A high-performance Go tool for managing LLM conversations as directed acyclic graphs (DAGs).

## Overview

LangDAG provides a simple yet powerful way to orchestrate LLM interactions by modeling conversations as DAGs. Each node represents an LLM call or tool execution, and edges define the flow of context between them.

### Goals

- **Performance first**: Written in Go for minimal latency and high throughput
- **Simple mental model**: DAGs are intuitive - nodes process, edges connect
- **Provider agnostic**: Start with Anthropic, extend to any provider
- **Flexible deployment**: CLI for development, API for production
- **First-class tool support**: Native handling of tool calls with streaming interruption

### Non-Goals

- **No magic memory**: No auto-summarization or semantic search. Summarization is just a node—start a new conversation with a summary node's output as context.
- **No built-in RAG**: Need retrieval? Add it as a tool. LangDAG orchestrates, not embeds.
- **Not a prompt management system**
- **Not a fine-tuning platform**

---

## Core Concepts

### DAG (Directed Acyclic Graph)

A DAG represents a complete conversation or workflow. It contains:

- **Nodes**: Individual LLM calls, tool executions, or control flow points
- **Edges**: Connections that pass context/output from one node to another
- **State**: Accumulated context that flows through the graph

```
┌─────────┐     ┌─────────┐     ┌─────────┐
│  Input  │────▶│  LLM 1  │────▶│  Tool   │
└─────────┘     └─────────┘     └─────────┘
                    │               │
                    │               ▼
                    │          ┌─────────┐
                    └─────────▶│  LLM 2  │
                               └─────────┘
```

### Two Modes: Workflows vs Conversations

LangDAG supports two fundamental patterns. Both are DAGs under the hood—the difference is how they're created.

#### Workflow Mode (Static DAG)

Pre-defined pipelines where the structure is known ahead of time. Defined in YAML.

```
┌─────────┐     ┌──────────┐     ┌────────────┐     ┌─────────┐
│  Input  │────▶│ Planner  │────▶│ Researcher │────▶│ Output  │
└─────────┘     └──────────┘     └────────────┘     └─────────┘

Use cases:
- Research pipelines
- Document processing
- Multi-step agents with known flow
- Batch processing
```

#### Conversation Mode (Dynamic DAG)

Live chat sessions where the DAG grows with each turn. Nodes are added dynamically as the conversation progresses.

```
Turn 1:              Turn 2:                  Turn 3 (with tool):

┌────────┐           ┌────────┐              ┌────────┐
│  user  │           │  user  │              │  user  │
└───┬────┘           └───┬────┘              └───┬────┘
    │                    │                       │
    ▼                    ▼                       ▼
┌────────┐           ┌────────┐              ┌────────┐
│  llm   │           │  llm   │              │  llm   │
└────────┘           └───┬────┘              └───┬────┘
                         │                       │
                         ▼                       ▼
                     ┌────────┐              ┌────────┐
                     │  user  │              │  tool  │ ← tool_call
                     └───┬────┘              └───┬────┘
                         │                       │
                         ▼                       ▼
                     ┌────────┐              ┌────────┐
                     │  llm   │              │  llm   │ ← continues
                     └────────┘              └───┬────┘
                                                 │
                                                 ▼
                                             ┌────────┐
                                             │  user  │
                                             └───┬────┘
                                                 │
                                                 ▼
                                             ┌────────┐
                                             │  llm   │
                                             └────────┘

Use cases:
- Chat interfaces
- Interactive assistants
- Agentic loops with human-in-the-loop
- Support bots
```

#### Key Insight

A conversation IS a DAG—it just grows incrementally:

| Aspect | Workflow | Conversation |
|--------|----------|--------------|
| DAG creation | Defined upfront in YAML | Built dynamically per turn |
| Structure | Fixed | Grows with each message |
| Nodes added | Never (immutable) | On every user/llm/tool turn |
| Branching | Explicit `branch` nodes | Implicit (tool calls, user choices) |
| Persistence | Definition + runs | Full message history as nodes |
| Replay | Re-run entire workflow | Fork from any point |

Both modes share the same:
- Storage schema (nodes, edges, state)
- Execution engine
- Tool handling
- Streaming infrastructure
- Provider interface

### Node Types

| Type | Description |
|------|-------------|
| `llm` | Makes a call to an LLM provider |
| `tool` | Executes a tool/function call |
| `branch` | Conditional routing based on output |
| `merge` | Combines outputs from multiple nodes |
| `input` | Entry point with user input |
| `output` | Terminal node with final result |

### Edges

Edges define data flow and can optionally transform data:

```yaml
edges:
  - from: node_a
    to: node_b
    transform: "$.response.content"  # JSONPath extraction (optional)
```

### State

State is a JSON object that accumulates as the DAG executes:

```json
{
  "input": "user query",
  "node_a_output": { ... },
  "node_b_output": { ... },
  "final": "result"
}
```

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                         CLI / API                            │
├──────────────────────────────────────────────────────────────┤
│                      DAG Executor                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │   Parser    │  │  Scheduler  │  │   Runner    │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
├──────────────────────────────────────────────────────────────┤
│                    Provider Layer                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │  Anthropic  │  │   OpenAI    │  │   Ollama    │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
├──────────────────────────────────────────────────────────────┤
│                    Storage Layer                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │   SQLite    │  │  PostgreSQL │  │    Redis    │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
└──────────────────────────────────────────────────────────────┘
```

### Components

**DAG Executor**
- **Parser**: Reads DAG definitions (YAML/JSON) and validates structure
- **Scheduler**: Determines execution order, handles parallelism
- **Runner**: Executes nodes, manages state, handles errors

**Provider Layer**
- Unified interface for all LLM providers
- Handles streaming, retries, rate limiting
- Provider-specific optimizations (batching, caching)

**Storage Layer**
- Persists DAG definitions and execution history
- Stores node outputs for replay/debugging
- Supports multiple backends

---

## Storage

### Primary: SQLite (Default)

SQLite is the default storage backend for simplicity and zero-configuration:

```
langdag/
├── langdag.db          # Main database
└── blobs/              # Large outputs (optional)
```

**Schema:**

```sql
-- DAG definitions (for Workflow mode)
CREATE TABLE dags (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version INTEGER DEFAULT 1,
    definition JSON NOT NULL,           -- YAML parsed to JSON
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Conversations (for Conversation mode)
-- A conversation IS a DAG, but built dynamically
CREATE TABLE conversations (
    id TEXT PRIMARY KEY,
    title TEXT,                          -- Auto-generated or user-set
    model TEXT NOT NULL,                 -- Default model for this conversation
    system_prompt TEXT,                  -- System prompt
    tools JSON,                          -- Available tools
    forked_from_conv TEXT REFERENCES conversations(id),
    forked_from_node TEXT,               -- Node ID where fork happened
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Nodes in a conversation (the actual DAG structure)
CREATE TABLE conversation_nodes (
    id TEXT PRIMARY KEY,
    conversation_id TEXT REFERENCES conversations(id) ON DELETE CASCADE,
    parent_id TEXT REFERENCES conversation_nodes(id),  -- Previous node (forms the DAG edge)
    sequence INTEGER NOT NULL,           -- Order in conversation
    node_type TEXT NOT NULL,             -- 'user', 'assistant', 'tool_call', 'tool_result'
    content JSON NOT NULL,               -- Message content, tool call, or tool result
    model TEXT,                          -- Model used (for assistant nodes)
    tokens_in INTEGER,
    tokens_out INTEGER,
    latency_ms INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Workflow execution runs
CREATE TABLE runs (
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
CREATE TABLE node_runs (
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
CREATE INDEX idx_runs_dag ON runs(dag_id);
CREATE INDEX idx_runs_status ON runs(status);
CREATE INDEX idx_node_runs_run ON node_runs(run_id);
CREATE INDEX idx_conv_nodes_conv ON conversation_nodes(conversation_id);
CREATE INDEX idx_conv_nodes_parent ON conversation_nodes(parent_id);
CREATE INDEX idx_conversations_fork ON conversations(forked_from_conv);
```

### Why SQLite First?

1. **Zero setup**: Single file, no server
2. **Fast**: Excellent read performance for DAG lookups
3. **Portable**: Easy to backup, share, inspect
4. **Sufficient**: Handles thousands of DAGs and millions of runs
5. **WAL mode**: Concurrent reads during writes

### Future Storage Options

| Backend | Use Case |
|---------|----------|
| PostgreSQL | Multi-user, high concurrency |
| Redis | Caching, real-time state |
| S3/GCS | Large blob storage |

---

## Provider Interface

### Unified Provider API

```go
type Provider interface {
    // Basic completion
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

    // Streaming completion
    Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error)

    // Tool execution support
    CompleteWithTools(ctx context.Context, req *ToolRequest) (*ToolResponse, error)

    // Provider info
    Name() string
    Models() []ModelInfo
}

type CompletionRequest struct {
    Model       string
    Messages    []Message
    System      string
    MaxTokens   int
    Temperature float64
    StopSeqs    []string
}

type Message struct {
    Role    string      // "user", "assistant", "tool_result"
    Content interface{} // string or []ContentBlock
}
```

### Anthropic Provider (Initial)

Supported models:
- `claude-sonnet-4-20250514`
- `claude-opus-4-20250514`
- `claude-haiku-3-5-20241022`

Features:
- Full Messages API support
- Native streaming with SSE
- Tool use with automatic continuation
- Vision (image inputs)
- Extended thinking (when available)

---

## Tool Calls

### Definition

Tools are defined at the DAG or node level:

```yaml
tools:
  - name: get_weather
    description: Get current weather for a location
    input_schema:
      type: object
      properties:
        location:
          type: string
          description: City name or coordinates
      required: [location]
```

### Execution Modes

**1. Automatic (Default)**

LangDAG automatically executes tool calls and continues the conversation:

```
User Input → LLM → Tool Call → Tool Execution → LLM → Response
```

**2. Interrupt**

Pause execution and return control for external tool handling:

```go
run, err := dag.Execute(ctx, input, WithToolInterrupt())
if run.Status == "tool_pending" {
    toolResult := executeExternally(run.PendingTool)
    run.Resume(ctx, toolResult)
}
```

**3. WebSocket Bidirectional**

Real-time streaming with tool interruption over WebSocket:

```
Client                          Server
   │                               │
   │──── execute(dag, input) ─────▶│
   │                               │
   │◀──── stream: tokens ──────────│
   │◀──── stream: tool_call ───────│
   │                               │
   │──── tool_result ─────────────▶│
   │                               │
   │◀──── stream: tokens ──────────│
   │◀──── stream: done ────────────│
```

---

## CLI Interface

### Commands

```bash
# DAG Management (Workflow Mode)
langdag init                      # Initialize new project
langdag dag list                  # List all DAGs
langdag dag create <name>         # Create new DAG from YAML
langdag dag show <id>             # Show DAG definition
langdag dag validate <file>       # Validate DAG file
langdag dag delete <id>           # Delete a DAG

# Workflow Execution
langdag run <dag> [--input JSON]  # Execute a workflow DAG
langdag run <dag> --stream        # Stream output
langdag run <dag> --dry-run       # Validate without executing

# Conversation Mode
langdag chat new                  # Start new conversation
langdag chat continue <conv-id>   # Continue existing conversation
langdag chat list                 # List all conversations
langdag chat show <conv-id>       # Show conversation as DAG
langdag chat fork <conv-id> <node># Fork conversation from specific node
langdag chat delete <conv-id>     # Delete conversation

# History (works for both modes)
langdag history list [--dag ID]   # List past runs
langdag history show <run-id>     # Show run details
langdag history replay <run-id>   # Replay a run
langdag history nodes <run-id>    # Show all nodes in a run

# Configuration
langdag config set <key> <value>  # Set config value
langdag config get <key>          # Get config value
langdag provider add anthropic    # Configure provider
```

### Example Session

```bash
$ langdag dag create summarizer
Created DAG: summarizer (id: dag_abc123)

$ cat summarizer.yaml
name: summarizer
nodes:
  - id: input
    type: input
  - id: summarize
    type: llm
    model: claude-sonnet-4-20250514
    system: "You are a concise summarizer."
    prompt: "Summarize: {{input}}"
  - id: output
    type: output
edges:
  - from: input
    to: summarize
  - from: summarize
    to: output

$ langdag run summarizer --input '{"text": "Long article..."}'
Running DAG: summarizer
├─ input: received
├─ summarize: streaming...
│  The article discusses three main points...
└─ output: complete

Result: {"summary": "The article discusses..."}
Run ID: run_xyz789
```

### Example: Conversation Mode

```bash
$ langdag chat new --model claude-sonnet-4-20250514
Starting new conversation (id: conv_abc123)
System prompt (optional, press Enter to skip): You are a helpful coding assistant.

You> What's the best way to parse JSON in Go?

Assistant> In Go, you have several options for parsing JSON...
[streaming response]

You> Can you show me an example with error handling?

Assistant> Here's a complete example:
```go
...
```

You> /tools add web_search    # Add tool mid-conversation
Tool 'web_search' added to conversation.

You> Search for the latest Go 1.22 JSON features

Assistant> I'll search for that information.
[tool_call: web_search("Go 1.22 JSON features")]

Tool result received. Continuing...

Assistant> According to the search results, Go 1.22 introduced...

You> /show                    # Show conversation DAG
Conversation: conv_abc123 (6 nodes)
├─ node_1 [user]: "What's the best way to parse JSON..."
├─ node_2 [llm]: "In Go, you have several options..."
├─ node_3 [user]: "Can you show me an example..."
├─ node_4 [llm]: "Here's a complete example..."
├─ node_5 [user]: "Search for the latest Go 1.22..."
├─ node_6 [llm]: tool_call(web_search)
├─ node_7 [tool]: web_search result
└─ node_8 [llm]: "According to the search results..."

You> /fork node_4             # Fork from node_4 to try different path
Forked conversation: conv_def456 (from node_4)

You> Actually, show me how to use json.Decoder instead

Assistant> Sure! json.Decoder is great for streaming...
```

---

## API Interface

### REST API

```
# Workflow DAGs
POST   /api/v1/dags                 # Create DAG
GET    /api/v1/dags                 # List DAGs
GET    /api/v1/dags/:id             # Get DAG
PUT    /api/v1/dags/:id             # Update DAG
DELETE /api/v1/dags/:id             # Delete DAG
POST   /api/v1/dags/:id/run         # Execute workflow DAG

# Conversations
POST   /api/v1/conversations                    # Start new conversation
GET    /api/v1/conversations                    # List conversations
GET    /api/v1/conversations/:id                # Get conversation (as DAG)
DELETE /api/v1/conversations/:id                # Delete conversation
POST   /api/v1/conversations/:id/messages       # Add message (continues conversation)
POST   /api/v1/conversations/:id/fork/:node_id  # Fork from specific node
GET    /api/v1/conversations/:id/nodes          # List all nodes in conversation

# Runs (shared)
GET    /api/v1/runs                 # List runs
GET    /api/v1/runs/:id             # Get run details
POST   /api/v1/runs/:id/resume      # Resume with tool result
DELETE /api/v1/runs/:id             # Cancel run
```

### WebSocket API

```
WS /api/v1/ws

# Client → Server (Workflow mode)
{"type": "execute", "dag_id": "...", "input": {...}}
{"type": "tool_result", "run_id": "...", "result": {...}}
{"type": "cancel", "run_id": "..."}

# Client → Server (Conversation mode)
{"type": "chat.new", "model": "...", "system": "..."}
{"type": "chat.message", "conversation_id": "...", "content": "..."}
{"type": "chat.fork", "conversation_id": "...", "from_node": "..."}
{"type": "tool_result", "conversation_id": "...", "result": {...}}

# Server → Client (shared)
{"type": "token", "content": "...", "node_id": "..."}
{"type": "tool_call", "tool": {...}, "node_id": "..."}
{"type": "node_complete", "node_id": "...", "node_type": "...", "output": {...}}
{"type": "complete", "output": {...}}
{"type": "error", "error": "..."}

# Server → Client (Conversation-specific)
{"type": "chat.created", "conversation_id": "..."}
{"type": "chat.node_added", "conversation_id": "...", "node": {...}}
{"type": "chat.forked", "conversation_id": "...", "from_conversation": "...", "from_node": "..."}
```

---

## DAG Definition Format

### YAML Schema

```yaml
# metadata
name: string                    # Required: DAG name
version: integer                # Optional: version number
description: string             # Optional: description

# provider defaults
defaults:
  provider: anthropic           # Default provider
  model: claude-sonnet-4-20250514  # Default model
  max_tokens: 4096              # Default max tokens
  temperature: 0.7              # Default temperature

# tool definitions
tools:
  - name: string
    description: string
    input_schema: object        # JSON Schema

# node definitions
nodes:
  - id: string                  # Required: unique identifier
    type: string                # Required: node type

    # LLM node specific
    model: string               # Override default model
    system: string              # System prompt
    prompt: string              # User prompt template
    tools: [string]             # Tool names this node can use

    # Branch node specific
    condition: string           # JSONPath expression

    # Tool node specific
    handler: string             # Tool handler reference

# edge definitions
edges:
  - from: string                # Source node ID
    to: string                  # Target node ID
    condition: string           # Optional: condition for edge
    transform: string           # Optional: JSONPath transform
```

### Example: Multi-step Research Agent

```yaml
name: research_agent
description: Research a topic and produce a structured report

defaults:
  model: claude-sonnet-4-20250514
  max_tokens: 4096

tools:
  - name: web_search
    description: Search the web for information
    input_schema:
      type: object
      properties:
        query: { type: string }
      required: [query]

  - name: fetch_url
    description: Fetch content from a URL
    input_schema:
      type: object
      properties:
        url: { type: string }
      required: [url]

nodes:
  - id: input
    type: input

  - id: planner
    type: llm
    system: "You are a research planner. Break down research queries into steps."
    prompt: "Create a research plan for: {{input.query}}"

  - id: researcher
    type: llm
    system: "You are a researcher. Use tools to gather information."
    prompt: "Execute this research plan:\n{{planner.output}}"
    tools: [web_search, fetch_url]

  - id: synthesizer
    type: llm
    system: "You synthesize research into clear reports."
    prompt: |
      Based on this research:
      {{researcher.output}}

      Create a structured report.

  - id: output
    type: output

edges:
  - from: input
    to: planner
  - from: planner
    to: researcher
  - from: researcher
    to: synthesizer
  - from: synthesizer
    to: output
```

---

## Configuration

### Config File

Location: `~/.config/langdag/config.yaml` or `./langdag.yaml`

```yaml
# Storage
storage:
  driver: sqlite                # sqlite, postgres, redis
  path: ./langdag.db            # For sqlite
  # connection: postgres://...  # For postgres

# Providers
providers:
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
    base_url: https://api.anthropic.com  # Optional

  openai:
    api_key: ${OPENAI_API_KEY}

# Server
server:
  host: 0.0.0.0
  port: 8080
  cors_origins: ["*"]

# Logging
logging:
  level: info                   # debug, info, warn, error
  format: text                  # text, json

# Execution
execution:
  default_timeout: 300s         # Per-node timeout
  max_parallel: 10              # Max parallel node executions
  retry_attempts: 3             # Auto-retry on transient errors
```

### Environment Variables

```bash
LANGDAG_CONFIG=/path/to/config.yaml
LANGDAG_STORAGE_PATH=./langdag.db
ANTHROPIC_API_KEY=sk-ant-...
OPENAI_API_KEY=sk-...
```

---

## Comparison

| Feature | LangDAG | LangGraph | Langfuse |
|---------|---------|-----------|----------|
| Language | Go | Python | TypeScript |
| Primary Focus | DAG execution | Agent graphs | Observability |
| Latency | ~1ms overhead | ~10-50ms | N/A (tracing) |
| Memory | ~10MB | ~100MB+ | N/A |
| Complexity | Low | Medium-High | Medium |
| Self-hosted | Yes (single binary) | Yes | Yes (Docker) |
| Streaming | Native | Via callbacks | Via SDK |
| Tool interruption | Native | Manual | N/A |

### Key Differentiators

1. **Performance**: Go's efficiency means minimal overhead per request
2. **Simplicity**: DAGs are the only abstraction - no agents, chains, or runnables
3. **Single binary**: No runtime dependencies, easy deployment
4. **Native streaming**: First-class SSE and WebSocket support
5. **SQL-first storage**: Simple, queryable, portable

---

## Roadmap

### Phase 1: Core (MVP)
- [ ] SQLite storage (conversations + workflows)
- [ ] Anthropic provider with streaming
- [ ] **Conversation mode**: chat new, continue, list, show
- [ ] Basic workflow mode: DAG parser, validator
- [ ] CLI for both modes
- [ ] Streaming output

### Phase 2: Tools & Forking
- [ ] Tool call support (auto + interrupt modes)
- [ ] Conversation forking from any node
- [ ] Tool result handling
- [ ] Run history and replay

### Phase 3: API Layer
- [ ] REST API (conversations + workflows)
- [ ] WebSocket streaming
- [ ] Tool interruption over WebSocket

### Phase 4: Extended Providers
- [ ] OpenAI provider
- [ ] Ollama provider (local models)
- [ ] Provider fallback chains

### Phase 5: Advanced Features
- [ ] PostgreSQL storage
- [ ] Redis caching
- [ ] Parallel node execution (workflows)
- [ ] Conditional branching (workflows)
- [ ] Web UI (optional)

---

## License

MIT
