<p align="center">
  <img src="docs/assets/langdag-banner.svg" width="600" alt="LangDAG">
</p>

<h3 align="center"><em>LLM Conversations as Directed Acyclic Graphs</em></h3>

<p align="center">
  <a href="https://github.com/yourusername/langdag/releases"><img src="https://img.shields.io/badge/version-0.2.0-00ADD8?style=flat-square" alt="Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"></a>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#documentation">Docs</a> •
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
git clone https://github.com/yourusername/langdag.git
cd langdag
go build -o langdag ./cmd/langdag
```

### Go Install

```bash
go install github.com/yourusername/langdag/cmd/langdag@latest
```

---

## Quick Start

### 1. Configure your provider

```bash
export ANTHROPIC_API_KEY="your-api-key"
```

### 2. Start a conversation

```bash
langdag chat new --model claude-sonnet-4-20250514
```

### 3. Or run a workflow

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
langdag workflow create research.yaml
langdag workflow run research --input '{"query": "quantum computing"}'
```

---

## CLI Reference

### Conversations

```bash
langdag chat new                    # Start new conversation
langdag chat new --system "..."     # With system prompt
langdag chat continue <id>          # Continue conversation
langdag chat continue <id> --node X # Fork from specific node
```

### DAG Management

```bash
langdag ls                          # List all DAGs
langdag show <id>                   # Show DAG details
langdag print <id>                  # Print DAG tree
langdag rm <id>                     # Delete DAG
```

### Workflows

```bash
langdag workflow create <file>      # Create from YAML
langdag workflow list               # List workflows
langdag workflow run <name>         # Execute workflow
langdag workflow run <name> --stream # With streaming
langdag workflow validate <file>    # Validate YAML
```

### Configuration

```bash
langdag config set <key> <value>    # Set config value
langdag config get <key>            # Get config value
```

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
- **[API Reference](https://yourusername.github.io/langdag/api)** — REST & WebSocket APIs
- **[Examples](examples/)** — Sample workflows

---

## Roadmap

- [x] SQLite storage
- [x] Anthropic provider with streaming
- [x] Conversation mode (new, continue, fork)
- [x] Workflow mode (YAML, validation, execution)
- [x] Tree visualization
- [ ] REST API
- [ ] WebSocket streaming
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
