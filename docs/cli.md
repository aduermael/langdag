---
layout: default
title: CLI Reference
nav_order: 3
---

# CLI Reference
{: .no_toc }

Complete reference for all LangDAG commands.
{: .fs-6 .fw-300 }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Global Options

```bash
langdag [command] [flags]
```

| Flag | Description |
|:-----|:------------|
| `--config` | Path to config file (default: ~/.config/langdag/config.yaml) |
| `--output` | Output format: `table`, `json`, `yaml` (default: table) |
| `--help` | Show help for any command |

---

## DAG Commands

### `langdag ls`

List all DAG instances (conversations and workflow runs).

```bash
langdag ls [flags]
```

| Flag | Description |
|:-----|:------------|
| `--type` | Filter by type: `conversation`, `workflow` |
| `--limit` | Maximum number of results (default: 50) |

**Example:**
```bash
langdag ls --type conversation --limit 10
```

---

### `langdag show`

Show details of a specific DAG.

```bash
langdag show <dag-id> [flags]
```

| Flag | Description |
|:-----|:------------|
| `--nodes` | Include all nodes in output |

**Example:**
```bash
langdag show dag_abc123 --nodes
```

---

### `langdag print`

Print the DAG as a tree structure with branches.

```bash
langdag print <dag-id>
```

**Example:**
```bash
langdag print dag_abc123
```

**Output:**
```
dag_abc123 (conversation)
├── [1] user: "Hello"
│   └── [2] assistant: "Hi! How can I help?"
│       ├── [3] user: "What's 2+2?"
│       │   └── [4] assistant: "4"
│       └── [5] user: "What's 3+3?" (branch)
│           └── [6] assistant: "6"
```

---

### `langdag rm`

Delete a DAG instance.

```bash
langdag rm <dag-id> [flags]
```

| Flag | Description |
|:-----|:------------|
| `--force` | Skip confirmation prompt |

---

## Chat Commands

### `langdag chat new`

Start a new conversation.

```bash
langdag chat new [flags]
```

| Flag | Description |
|:-----|:------------|
| `--model` | LLM model to use (default: claude-sonnet-4-20250514) |
| `--system` | System prompt |
| `--title` | Conversation title |

**Examples:**
```bash
# Basic conversation
langdag chat new

# With specific model
langdag chat new --model claude-opus-4-20250514

# With system prompt
langdag chat new --system "You are a Python expert"
```

---

### `langdag chat continue`

Continue an existing conversation.

```bash
langdag chat continue <dag-id> [flags]
```

| Flag | Description |
|:-----|:------------|
| `--node` | Fork from specific node ID |

**Examples:**
```bash
# Continue from last message
langdag chat continue dag_abc123

# Fork from node 5
langdag chat continue dag_abc123 --node 5
```

---

## Workflow Commands

### `langdag workflow create`

Create a new workflow from a YAML file.

```bash
langdag workflow create <file> [flags]
```

**Example:**
```bash
langdag workflow create ./my-workflow.yaml
```

---

### `langdag workflow list`

List all workflow templates.

```bash
langdag workflow list
```

---

### `langdag workflow show`

Show workflow definition.

```bash
langdag workflow show <workflow-id>
```

---

### `langdag workflow run`

Execute a workflow.

```bash
langdag workflow run <name> [flags]
```

| Flag | Description |
|:-----|:------------|
| `--input` | JSON input data |
| `--stream` | Stream output in real-time |
| `--dry-run` | Validate without executing |

**Examples:**
```bash
# Basic run
langdag workflow run summarizer --input '{"text": "..."}'

# With streaming
langdag workflow run summarizer --input '{"text": "..."}' --stream

# Dry run (validation only)
langdag workflow run summarizer --dry-run
```

---

### `langdag workflow validate`

Validate a workflow YAML file.

```bash
langdag workflow validate <file>
```

---

### `langdag workflow delete`

Delete a workflow template.

```bash
langdag workflow delete <workflow-id>
```

---

## Config Commands

### `langdag config set`

Set a configuration value.

```bash
langdag config set <key> <value>
```

**Examples:**
```bash
langdag config set providers.anthropic.api_key "sk-ant-..."
langdag config set storage.path "./data/langdag.db"
langdag config set logging.level "debug"
```

---

### `langdag config get`

Get a configuration value.

```bash
langdag config get <key>
```

**Example:**
```bash
langdag config get providers.anthropic.api_key
```

---

## Other Commands

### `langdag version`

Show version information.

```bash
langdag version
```

---

## Exit Codes

| Code | Description |
|:-----|:------------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Configuration error |
| 4 | Provider error |
| 5 | Storage error |
