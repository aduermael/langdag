<p align="center">
  <img src="docs/assets/langdag-banner.svg" width="600" alt="LangDAG">
</p>

<h3 align="center"><em>LLM Conversations as Directed Acyclic Graphs</em></h3>

<p align="center">
  <a href="https://github.com/aduermael/langdag/actions/workflows/test-go.yml"><img src="https://img.shields.io/github/actions/workflow/status/aduermael/langdag/test-go.yml?style=flat-square&label=Go%20SDK" alt="Go SDK Tests"></a>
  <a href="https://github.com/aduermael/langdag/actions/workflows/test-python.yml"><img src="https://img.shields.io/github/actions/workflow/status/aduermael/langdag/test-python.yml?style=flat-square&label=Python%20SDK" alt="Python SDK Tests"></a>
  <a href="https://github.com/aduermael/langdag/actions/workflows/test-typescript.yml"><img src="https://img.shields.io/github/actions/workflow/status/aduermael/langdag/test-typescript.yml?style=flat-square&label=TypeScript%20SDK" alt="TypeScript SDK Tests"></a>
  <a href="https://github.com/aduermael/langdag/actions/workflows/test.yml"><img src="https://img.shields.io/github/actions/workflow/status/aduermael/langdag/test.yml?style=flat-square&label=E2E" alt="E2E Tests"></a>
  <a href="https://github.com/aduermael/langdag/releases"><img src="https://img.shields.io/badge/version-0.2.0-00ADD8?style=flat-square" alt="Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"></a>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#cli">CLI</a> •
  <a href="#api--sdks">API & SDKs</a> •
  <a href="#roadmap">Roadmap</a>
</p>

---

## Why LangDAG?

LangDAG is a **high-performance Go tool** that models LLM conversations and workflows as directed acyclic graphs. Whether you're building chatbots, AI agents, or complex multi-step pipelines—LangDAG provides a unified, powerful abstraction.

```
┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
│  User   │────▶│   LLM   │────▶│  Tool   │────▶│   LLM   │
└─────────┘     └─────────┘     └─────────┘     └─────────┘
                    │                               │
                    └──────── conversation ─────────┘
                                 = DAG
```

**Key insight:** A conversation *is* a DAG—it just grows incrementally.

---

## Features

| Feature | Description |
|---------|-------------|
| 🔀 **Two Modes** | **Workflow** (YAML pipelines) + **Conversation** (dynamic chat) — same engine |
| ⚡ **Performance** | Pure Go, ~1ms overhead, single static binary, zero runtime deps |
| 🌊 **Native Streaming** | SSE + WebSocket support with tool call interruption |
| 🔧 **Tool Integration** | Auto, Interrupt, or WebSocket execution modes |
| 🌳 **Conversation Forking** | Branch from any node, explore alternative paths |
| 💾 **Persistent Storage** | SQLite (default) or PostgreSQL, full history replay |

---

## Installation

### From Source

```bash
git clone https://github.com/aduermael/langdag.git
cd langdag
go build -o langdag ./cmd/langdag
```

### Go Install

```bash
go install github.com/aduermael/langdag/cmd/langdag@latest
```

---

## CLI

```bash
# Set your API key
export ANTHROPIC_API_KEY="your-api-key"

# Start a new conversation
langdag prompt "What is a DAG?"

# Interactive mode
langdag prompt

# List all conversations (root nodes)
langdag ls

# View a conversation tree
langdag show a1b2

# Continue from a specific node
langdag prompt a1b2 "Tell me more"

# Delete a conversation
langdag rm a1b2
```

Every conversation is a DAG that grows as you chat—and you can branch from any point:

```
Node history:
├─ 1a2b [human]: What is a DAG?
│  ├─ 5e6f [assistant]: A DAG is a directed graph with no cycles...
│  │  └─ 9c0d [human]: Can you give me an example?
│  │     └─ 2f34 [assistant]: Sure! Think of a family tree...
│  └─ 7a8b [assistant]: Let me explain with a diagram...
│     └─ 4e5f [human]: That's clearer, thanks!
└─ ...
```

<details>
<summary><strong>CLI Reference</strong></summary>

```bash
# Prompt commands
langdag prompt "message"               # Start new conversation
langdag prompt -m <model> "message"    # Use a specific model
langdag prompt -s "system" "message"   # With system prompt
langdag prompt <node-id> "message"     # Continue from node
langdag prompt                         # Interactive mode (new tree)
langdag prompt <node-id>               # Interactive mode from node

# Node management
langdag ls                             # List root nodes
langdag show <id>                      # Show node tree
langdag rm <id>                        # Delete node and subtree
```

</details>

---

## API & SDKs

LangDAG can run as a REST API server:

```bash
langdag serve --port 8080
```

**Endpoints:**
- `POST /prompt` — Start new conversation tree
- `POST /nodes/{id}/prompt` — Continue from existing node
- `GET /nodes` — List root nodes
- `GET /nodes/{id}` — Get a single node
- `GET /nodes/{id}/tree` — Get full tree from node
- `DELETE /nodes/{id}` — Delete node and subtree
- `POST /workflows/{id}/run` — Execute a workflow

See the [OpenAPI specification](api/openapi.yaml) for full API documentation.

### Python

```bash
pip install langdag
```

```python
from langdag import LangDAGClient

client = LangDAGClient()

# Start a conversation
node = client.prompt("What is a DAG?")
print(node.content)

# Continue from any node
node2 = node.prompt("Tell me more")

# Stream responses
for event in client.prompt("Explain graphs", stream=True):
    if event.content:
        print(event.content, end="")
```

### Go

```bash
go get github.com/langdag/langdag-go
```

```go
client := langdag.NewClient("http://localhost:8080")

// Start a conversation
node, _ := client.Prompt(ctx, "What is a DAG?")
fmt.Println(node.Content)

// Continue from any node
node2, _ := node.Prompt(ctx, "Tell me more")

// Stream responses
stream, _ := client.PromptStream(ctx, "Explain graphs")
for event := range stream.Events() {
    fmt.Print(event.Content)
}
result, _ := stream.Node()
```

### TypeScript

```bash
npm install langdag
```

```typescript
import { LangDAGClient } from 'langdag';

const client = new LangDAGClient();

// Start a conversation
const node = await client.prompt('What is a DAG?');
console.log(node.content);

// Continue from any node
const node2 = await node.prompt('Tell me more');

// Stream responses
const stream = await client.promptStream('Explain graphs');
for await (const event of stream.events()) {
  process.stdout.write(event.content);
}
const result = await stream.node();
```

See the [SDK source code](sdks/) and [example projects](examples/) for more details.

---

## Workflows

For pre-defined pipelines, LangDAG supports YAML workflow definitions:

```yaml
# research.yaml
name: research_agent
description: Research a topic

defaults:
  model: claude-sonnet-4-20250514
  max_tokens: 4096

nodes:
  - id: input
    type: input
  - id: researcher
    type: llm
    system: "You are a research assistant."
    prompt: "Research this topic: {{input}}"
  - id: output
    type: output

edges:
  - from: input
    to: researcher
  - from: researcher
    to: output
```

```bash
# Create and run workflows
langdag workflow create research.yaml
langdag workflow run research --input '{"query": "quantum computing"}'

# Workflow management
langdag workflow list               # List workflows
langdag workflow run <name> --stream # With streaming
langdag workflow validate <file>    # Validate YAML
```

Workflows create node trees that can be continued interactively using `langdag prompt`.

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                      CLI / API                           │
├──────────────────────────────────────────────────────────┤
│                    DAG Executor                          │
│   ┌───────────┐   ┌───────────┐   ┌───────────┐         │
│   │  Parser   │   │ Scheduler │   │  Runner   │         │
│   └───────────┘   └───────────┘   └───────────┘         │
├──────────────────────────────────────────────────────────┤
│                   Provider Layer                         │
│   ┌───────────┐   ┌───────────┐   ┌───────────┐         │
│   │ Anthropic │   │  OpenAI   │   │  Ollama   │         │
│   └───────────┘   └───────────┘   └───────────┘         │
├──────────────────────────────────────────────────────────┤
│                   Storage Layer                          │
│   ┌───────────┐   ┌───────────┐   ┌───────────┐         │
│   │  SQLite   │   │ PostgreSQL│   │   Redis   │         │
│   └───────────┘   └───────────┘   └───────────┘         │
└──────────────────────────────────────────────────────────┘
```

---

## Documentation

- **[Design Document](docs/DESIGN.md)** — Deep dive into architecture
- **[API Reference](https://aduermael.github.io/langdag/api)** — REST & WebSocket APIs
- **[Examples](examples/)** — Sample workflows

---

## Roadmap

- [x] SQLite storage
- [x] Anthropic provider with streaming
- [x] Node-centric API (prompt, branch, tree)
- [x] Workflow mode (YAML, validation, execution)
- [x] Tree visualization
- [x] REST API with SSE streaming
- [x] Python, Go, TypeScript SDKs
- [ ] OpenAI & Ollama providers
- [ ] PostgreSQL storage
- [ ] Web UI

---

## Comparison

| | LangDAG | LangGraph | Langfuse |
|---|---------|-----------|----------|
| **Focus** | DAG orchestration | State machines | Observability |
| **Language** | Go | Python | TypeScript |
| **Performance** | ~1ms overhead | Higher latency | N/A |
| **Conversation model** | Native DAG | Manual | Trace-based |
| **Deployment** | Single binary | Python runtime | SaaS/Self-host |

---

## Contributing

Contributions are welcome! Please read the [Contributing Guide](CONTRIBUTING.md) first.

```bash
# Run tests
go test ./...

# Build
go build -o langdag ./cmd/langdag
```

---

## License

MIT License - see [LICENSE](LICENSE) for details.

---

<p align="center">
  <sub>Built with ❤️ in Go</sub>
</p>
