---
layout: default
title: Getting Started
nav_order: 2
---

# Getting Started
{: .no_toc }

Get up and running with LangDAG in minutes.
{: .fs-6 .fw-300 }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Installation

### Prerequisites

- Go 1.23 or higher
- An Anthropic API key (other providers coming soon)

### From Source

```bash
git clone https://github.com/yourusername/langdag.git
cd langdag
go build -o langdag ./cmd/langdag
```

### Using Go Install

```bash
go install github.com/yourusername/langdag/cmd/langdag@latest
```

### Verify Installation

```bash
langdag version
```

---

## Configuration

### Environment Variables

The quickest way to configure LangDAG:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

### Configuration File

Create `~/.config/langdag/config.yaml`:

```yaml
storage:
  driver: sqlite
  path: ~/.local/share/langdag/langdag.db

providers:
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}

logging:
  level: info
  format: text
```

### Using the CLI

```bash
langdag config set providers.anthropic.api_key "sk-ant-..."
langdag config get providers.anthropic.api_key
```

---

## Your First Conversation

Start an interactive chat session:

```bash
langdag chat new
```

With a specific model:

```bash
langdag chat new --model claude-sonnet-4-20250514
```

With a system prompt:

```bash
langdag chat new --system "You are a helpful coding assistant"
```

### Continue a Conversation

```bash
# List your conversations
langdag ls

# Continue by ID
langdag chat continue dag_abc123
```

### Fork a Conversation

Branch from a specific point to explore alternatives:

```bash
# Show conversation tree
langdag print dag_abc123

# Fork from node 5
langdag chat continue dag_abc123 --node 5
```

---

## Your First Workflow

### Create a Workflow File

Save as `summarizer.yaml`:

```yaml
name: summarizer
description: Summarize text input

defaults:
  model: claude-sonnet-4-20250514
  max_tokens: 4096

nodes:
  - id: input
    type: input

  - id: summarize
    type: llm
    system: "You are a concise summarizer. Provide clear, brief summaries."
    prompt: "Summarize the following text:\n\n{{input}}"

  - id: output
    type: output

edges:
  - from: input
    to: summarize
  - from: summarize
    to: output
```

### Register the Workflow

```bash
langdag workflow create summarizer.yaml
```

### Run the Workflow

```bash
langdag workflow run summarizer --input '{"text": "Your long text here..."}'
```

With streaming output:

```bash
langdag workflow run summarizer --input '{"text": "..."}' --stream
```

---

## Managing DAGs

### List All DAGs

```bash
langdag ls
```

Output:
```
ID           TYPE          MODEL                      CREATED              NODES
dag_abc123   conversation  claude-sonnet-4-20250514   2024-01-15 10:30    12
dag_def456   workflow      claude-sonnet-4-20250514   2024-01-15 11:00    4
```

### Show DAG Details

```bash
langdag show dag_abc123
```

### Print DAG Tree

```bash
langdag print dag_abc123
```

Output:
```
dag_abc123 (conversation)
├── [1] user: "What is the capital of France?"
│   └── [2] assistant: "The capital of France is Paris..."
│       └── [3] user: "What's the population?"
│           └── [4] assistant: "Paris has a population of..."
│               ├── [5] user: "Compare to London" (branch)
│               │   └── [6] assistant: "London's population..."
│               └── [7] user: "Tell me about landmarks"
│                   └── [8] assistant: "Paris is famous for..."
```

### Delete a DAG

```bash
langdag rm dag_abc123
```

---

## Next Steps

- [CLI Reference]({{ '/cli' | relative_url }}) — Complete command documentation
- [Workflows Guide]({{ '/workflows' | relative_url }}) — Advanced workflow patterns
- [API Documentation]({{ '/api' | relative_url }}) — REST and WebSocket APIs
- [Design Document]({{ '/design' | relative_url }}) — Architecture deep dive
