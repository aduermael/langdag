---
layout: default
title: Workflows Guide
nav_order: 5
---

# Workflows Guide
{: .no_toc }

Build powerful LLM pipelines with YAML workflows.
{: .fs-6 .fw-300 }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## What is a Workflow?

A workflow is a **pre-defined DAG** that describes a sequence of LLM calls, tool executions, and data transformations. Unlike conversations (which grow dynamically), workflows have a fixed structure defined in YAML.

```yaml
# A simple workflow
name: example
nodes:
  - id: input
    type: input
  - id: process
    type: llm
    prompt: "Process: {{input}}"
  - id: output
    type: output
edges:
  - from: input
    to: process
  - from: process
    to: output
```

---

## Workflow Structure

### Required Fields

```yaml
name: my-workflow           # Unique identifier
description: Does something  # Human-readable description

nodes: []                   # List of nodes
edges: []                   # List of edges
```

### Optional Fields

```yaml
defaults:                   # Default values for all nodes
  model: claude-sonnet-4-20250514
  max_tokens: 4096
  temperature: 0.7

tools: []                   # Tool definitions
```

---

## Node Types

### Input Node

Entry point for user data.

```yaml
- id: input
  type: input
```

Access input data in prompts with `{{input}}` or `{{input.field}}`.

---

### LLM Node

Makes a call to an LLM provider.

```yaml
- id: analyzer
  type: llm
  model: claude-sonnet-4-20250514    # Optional, uses default
  system: "You are an expert analyst"
  prompt: "Analyze: {{input.text}}"
  max_tokens: 4096
  temperature: 0.7
  tools: [search, calculate]         # Optional tool access
```

---

### Tool Node

Executes a tool/function.

```yaml
- id: search
  type: tool
  tool: web_search
  input:
    query: "{{previous_node.output}}"
```

---

### Branch Node

Conditional routing based on output.

```yaml
- id: router
  type: branch
  conditions:
    - when: "output contains 'error'"
      goto: error_handler
    - when: "output.score > 0.8"
      goto: high_quality
    - default: standard_path
```

---

### Merge Node

Combines outputs from multiple nodes.

```yaml
- id: combine
  type: merge
  inputs: [node_a, node_b, node_c]
  strategy: concat              # concat, json, or custom template
```

---

### Output Node

Terminal node with final result.

```yaml
- id: output
  type: output
```

---

## Edge Definitions

Edges connect nodes and can optionally transform data.

### Basic Edge

```yaml
edges:
  - from: input
    to: processor
```

### Edge with Transform

```yaml
edges:
  - from: analyzer
    to: summarizer
    transform: "$.analysis.key_points"  # JSONPath extraction
```

---

## Template Syntax

Use Handlebars-style templates to reference data:

```yaml
prompt: |
  Input: {{input}}
  Previous result: {{analyzer.output}}
  Specific field: {{analyzer.output.summary}}
```

### Available Variables

| Variable | Description |
|:---------|:------------|
| `{{input}}` | Original input data |
| `{{input.field}}` | Specific input field |
| `{{node_id.output}}` | Output from a previous node |
| `{{node_id.output.field}}` | Specific field from node output |

---

## Tool Definitions

Define tools that LLM nodes can use:

```yaml
tools:
  - name: web_search
    description: Search the web for information
    input_schema:
      type: object
      properties:
        query:
          type: string
          description: Search query
      required: [query]

  - name: calculate
    description: Perform mathematical calculations
    input_schema:
      type: object
      properties:
        expression:
          type: string
      required: [expression]
```

---

## Example Workflows

### Research Agent

```yaml
name: research_agent
description: Research a topic and produce a report

defaults:
  model: claude-sonnet-4-20250514
  max_tokens: 4096

tools:
  - name: web_search
    description: Search the web
    input_schema:
      type: object
      properties:
        query: { type: string }

nodes:
  - id: input
    type: input

  - id: planner
    type: llm
    system: "You are a research planner."
    prompt: |
      Create a research plan for the following topic.
      Include 3-5 search queries.

      Topic: {{input.topic}}

  - id: researcher
    type: llm
    system: "You are a thorough researcher."
    prompt: |
      Execute this research plan and gather information.

      Plan:
      {{planner.output}}
    tools: [web_search]

  - id: writer
    type: llm
    system: "You are a technical writer."
    prompt: |
      Write a comprehensive report based on this research.

      Research:
      {{researcher.output}}

  - id: output
    type: output

edges:
  - from: input
    to: planner
  - from: planner
    to: researcher
  - from: researcher
    to: writer
  - from: writer
    to: output
```

---

### Code Review Pipeline

```yaml
name: code_review
description: Review code and suggest improvements

defaults:
  model: claude-sonnet-4-20250514

nodes:
  - id: input
    type: input

  - id: security_check
    type: llm
    system: "You are a security expert."
    prompt: |
      Review this code for security vulnerabilities.

      ```
      {{input.code}}
      ```

  - id: style_check
    type: llm
    system: "You are a code style expert."
    prompt: |
      Review this code for style and best practices.

      ```
      {{input.code}}
      ```

  - id: combine
    type: merge
    inputs: [security_check, style_check]
    strategy: json

  - id: summarize
    type: llm
    prompt: |
      Summarize these code reviews into actionable feedback:

      Security: {{security_check.output}}
      Style: {{style_check.output}}

  - id: output
    type: output

edges:
  - from: input
    to: security_check
  - from: input
    to: style_check
  - from: security_check
    to: combine
  - from: style_check
    to: combine
  - from: combine
    to: summarize
  - from: summarize
    to: output
```

---

## Validation

Validate workflows before running:

```bash
langdag workflow validate my-workflow.yaml
```

Validation checks:
- All required fields present
- Node IDs are unique
- Edges reference valid nodes
- No cycles in the graph
- Tools are properly defined

---

## Execution

### Basic Run

```bash
langdag workflow run research_agent --input '{"topic": "quantum computing"}'
```

### Streaming Output

```bash
langdag workflow run research_agent --input '{"topic": "..."}' --stream
```

### Dry Run

```bash
langdag workflow run research_agent --dry-run
```

---

## Best Practices

{: .tip }
> **Keep workflows focused** — One workflow should do one thing well.

1. **Use descriptive node IDs** — `security_analyzer` not `node1`
2. **Set sensible defaults** — Avoid repeating model/token settings
3. **Add system prompts** — They improve LLM output quality
4. **Test with dry-run** — Validate before execution
5. **Use specific prompts** — Be explicit about expected output format
