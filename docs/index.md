---
layout: home
title: Home
nav_order: 1
description: "LangDAG - High-performance LLM conversation orchestration as directed acyclic graphs"
permalink: /
---

<div align="center" style="margin-bottom: 2rem;">
  <img src="{{ '/assets/langdag-banner.svg' | relative_url }}" alt="LangDAG" width="500">
</div>

# LangDAG
{: .fs-9 }

High-performance LLM conversation orchestration as directed acyclic graphs.
{: .fs-6 .fw-300 }

[Get Started](#quick-start){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/yourusername/langdag){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## Why LangDAG?

LangDAG provides a **unified abstraction** for managing LLM interactions. Every conversation is a DAG—nodes are messages or tool calls, edges define the flow.

```
┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
│  User   │────▶│   LLM   │────▶│  Tool   │────▶│   LLM   │
└─────────┘     └─────────┘     └─────────┘     └─────────┘
```

### Two Modes, One Engine

| Workflow Mode | Conversation Mode |
|:--------------|:------------------|
| Pre-defined YAML pipelines | Dynamic chat sessions |
| Structure known upfront | DAG grows per turn |
| Batch processing | Interactive assistants |

---

## Features

{: .highlight }
> **Performance First** — Written in Go with ~1ms overhead. Single static binary.

- **Native Streaming** — SSE and WebSocket support
- **Tool Integration** — Auto, Interrupt, or WebSocket execution modes
- **Conversation Forking** — Branch from any node
- **Persistent Storage** — SQLite default, PostgreSQL ready
- **Full Replay** — Debug any conversation step-by-step

---

## Demo

{% include terminal-demo.html %}

---

## Quick Start

### Install

```bash
git clone https://github.com/yourusername/langdag.git
cd langdag && go build -o langdag ./cmd/langdag
```

### Configure

```bash
export ANTHROPIC_API_KEY="your-api-key"
```

### Chat

```bash
langdag chat new --model claude-sonnet-4-20250514
```

### Run a Workflow

```bash
langdag workflow run summarizer --input '{"text": "..."}'
```

---

## Architecture Overview

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

## Example Workflow

```yaml
name: summarizer
description: Summarize any text

defaults:
  model: claude-sonnet-4-20250514
  max_tokens: 4096

nodes:
  - id: input
    type: input
  - id: summarize
    type: llm
    system: "You are a concise summarizer."
    prompt: "Summarize: {{input}}"
  - id: output
    type: output

edges:
  - from: input
    to: summarize
  - from: summarize
    to: output
```

---

## Learn More

<div class="code-example" markdown="1">

[Getting Started]({{ '/getting-started' | relative_url }}){: .btn .btn-outline }
[CLI Reference]({{ '/cli' | relative_url }}){: .btn .btn-outline }
[API Docs]({{ '/api' | relative_url }}){: .btn .btn-outline }
[Design Doc]({{ '/design' | relative_url }}){: .btn .btn-outline }

</div>

---

<div align="center" style="margin-top: 3rem; color: #666;">
  <small>Built with ❤️ in Go • MIT License</small>
</div>
