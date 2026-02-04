---
layout: default
title: Design Document
nav_order: 6
---

# Design Document

For the complete design document, see [DESIGN.md](https://github.com/yourusername/langdag/blob/main/docs/DESIGN.md).

{: .note }
> This page provides an overview. The full design document contains detailed architecture, schema definitions, and implementation details.

---

## Core Philosophy

LangDAG is built on a simple insight: **conversations are DAGs**.

Every chat message, every tool call, every LLM response—they're all nodes in a graph. The edges define how context flows between them.

```
┌─────────┐     ┌─────────┐     ┌─────────┐
│  User   │────▶│   LLM   │────▶│  Tool   │
└─────────┘     └─────────┘     └─────────┘
                    │               │
                    │               ▼
                    │          ┌─────────┐
                    └─────────▶│   LLM   │
                               └─────────┘
```

This abstraction unifies:
- Interactive chat sessions
- Pre-defined pipelines
- Agentic tool loops
- Conversation branching

---

## Two Modes, One Model

### Workflow Mode (Static DAG)

```yaml
# Defined upfront in YAML
nodes:
  - id: input
  - id: process
  - id: output
```

- Structure is fixed
- Nodes added at definition time
- Good for: Pipelines, batch processing

### Conversation Mode (Dynamic DAG)

```
Turn 1: User → LLM
Turn 2: User → LLM → Tool → LLM
Turn 3: Fork from Turn 1...
```

- Structure grows per turn
- Nodes added dynamically
- Good for: Chat, agents

**Same engine. Same storage. Same tools.**

---

## Architecture Layers

```
┌─────────────────────────────────────────────┐
│               CLI / REST API                │
├─────────────────────────────────────────────┤
│              DAG Executor                   │
│    Parser → Scheduler → Runner              │
├─────────────────────────────────────────────┤
│            Provider Layer                   │
│    Anthropic │ OpenAI │ Ollama              │
├─────────────────────────────────────────────┤
│            Storage Layer                    │
│    SQLite │ PostgreSQL │ Redis              │
└─────────────────────────────────────────────┘
```

---

## Key Design Decisions

### 1. Go for Performance

~1ms overhead per operation. Single static binary. No runtime dependencies.

### 2. SQLite by Default

Zero configuration. WAL mode for concurrent reads. Easy backup (single file).

### 3. First-Class Streaming

SSE and WebSocket support built-in. Tool calls can interrupt streams.

### 4. Immutable History

Nodes are never modified. Branching creates new nodes, preserving history.

---

## Storage Schema

```sql
-- DAG instances
CREATE TABLE dags (
    id TEXT PRIMARY KEY,
    workflow_id TEXT,           -- NULL for conversations
    model TEXT,
    system_prompt TEXT,
    status TEXT,
    created_at DATETIME,
    updated_at DATETIME
);

-- Nodes in DAGs
CREATE TABLE dag_nodes (
    id INTEGER PRIMARY KEY,
    dag_id TEXT,
    parent_id INTEGER,          -- Forms the DAG edges
    sequence INTEGER,
    node_type TEXT,             -- user, assistant, tool_call, etc.
    content TEXT,
    tokens_in INTEGER,
    tokens_out INTEGER,
    created_at DATETIME
);
```

---

## Provider Interface

```go
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error)
}
```

Implemented:
- Anthropic (claude-sonnet, claude-opus, claude-haiku)

Planned:
- OpenAI
- Ollama

---

## Tool Execution Modes

| Mode | Behavior |
|:-----|:---------|
| **Automatic** | Execute tools and continue LLM loop |
| **Interrupt** | Pause for external handling |
| **WebSocket** | Real-time bidirectional tool handling |

---

## What LangDAG is NOT

- **Not a prompt manager** — Use templates or dedicated tools
- **Not a RAG system** — Add retrieval as a tool
- **Not a fine-tuning platform** — Use provider tools
- **Not a magic memory system** — Summarization is just a node

LangDAG does one thing: **orchestrate LLM conversations as DAGs**.

---

[Read the full design document →](https://github.com/yourusername/langdag/blob/main/docs/DESIGN.md)
