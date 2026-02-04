---
layout: default
title: API Reference
nav_order: 4
---

# API Reference
{: .no_toc }

REST and WebSocket API documentation.
{: .fs-6 .fw-300 }

{: .warning }
> The REST API is currently under development. This documentation describes the planned API.

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## REST API

### Base URL

```
http://localhost:8080/api/v1
```

### Authentication

Include your API key in the `Authorization` header:

```bash
curl -H "Authorization: Bearer your-api-key" \
     http://localhost:8080/api/v1/dags
```

---

## Conversations

### Create Conversation

```http
POST /conversations
```

**Request Body:**
```json
{
  "model": "claude-sonnet-4-20250514",
  "system": "You are a helpful assistant",
  "title": "My Conversation"
}
```

**Response:**
```json
{
  "id": "dag_abc123",
  "model": "claude-sonnet-4-20250514",
  "system": "You are a helpful assistant",
  "created_at": "2024-01-15T10:30:00Z"
}
```

---

### Send Message

```http
POST /conversations/{id}/messages
```

**Request Body:**
```json
{
  "content": "What is the capital of France?"
}
```

**Response:**
```json
{
  "node_id": 2,
  "role": "assistant",
  "content": "The capital of France is Paris...",
  "tokens_in": 15,
  "tokens_out": 42
}
```

---

### Send Message (Streaming)

```http
POST /conversations/{id}/messages
Accept: text/event-stream
```

**Response (SSE):**
```
event: content_block_delta
data: {"type":"text_delta","text":"The"}

event: content_block_delta
data: {"type":"text_delta","text":" capital"}

event: message_stop
data: {}
```

---

### Fork Conversation

```http
POST /conversations/{id}/fork
```

**Request Body:**
```json
{
  "from_node": 5
}
```

**Response:**
```json
{
  "id": "dag_def456",
  "forked_from_dag": "dag_abc123",
  "forked_from_node": 5
}
```

---

### Get Conversation

```http
GET /conversations/{id}
```

**Query Parameters:**
- `include_nodes` (boolean): Include all nodes

**Response:**
```json
{
  "id": "dag_abc123",
  "model": "claude-sonnet-4-20250514",
  "nodes": [
    {
      "id": 1,
      "type": "user",
      "content": "Hello",
      "parent_id": null
    },
    {
      "id": 2,
      "type": "assistant",
      "content": "Hi! How can I help?",
      "parent_id": 1
    }
  ]
}
```

---

### List Conversations

```http
GET /conversations
```

**Query Parameters:**
- `limit` (int): Max results (default: 50)
- `offset` (int): Pagination offset

---

### Delete Conversation

```http
DELETE /conversations/{id}
```

---

## Workflows

### Create Workflow

```http
POST /workflows
Content-Type: application/yaml
```

**Request Body:**
```yaml
name: summarizer
description: Summarize text
nodes:
  - id: input
    type: input
  - id: summarize
    type: llm
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

### Run Workflow

```http
POST /workflows/{name}/run
```

**Request Body:**
```json
{
  "input": {
    "text": "Long text to summarize..."
  }
}
```

**Response:**
```json
{
  "dag_id": "dag_xyz789",
  "output": "Summary of the text...",
  "nodes_executed": 3,
  "total_tokens": 156
}
```

---

### Run Workflow (Streaming)

```http
POST /workflows/{name}/run
Accept: text/event-stream
```

---

### List Workflows

```http
GET /workflows
```

---

### Get Workflow

```http
GET /workflows/{name}
```

---

### Delete Workflow

```http
DELETE /workflows/{name}
```

---

## WebSocket API

### Connect

```
ws://localhost:8080/ws
```

### Message Format

All messages are JSON with a `type` field:

```json
{
  "type": "message_type",
  "data": { ... }
}
```

---

### Send Message

**Client → Server:**
```json
{
  "type": "message",
  "data": {
    "conversation_id": "dag_abc123",
    "content": "Hello"
  }
}
```

**Server → Client (streaming):**
```json
{
  "type": "content_delta",
  "data": {
    "text": "Hi"
  }
}
```

```json
{
  "type": "message_complete",
  "data": {
    "node_id": 2,
    "tokens_in": 5,
    "tokens_out": 20
  }
}
```

---

### Tool Call Handling

**Server → Client (tool call):**
```json
{
  "type": "tool_call",
  "data": {
    "id": "call_123",
    "name": "web_search",
    "input": {
      "query": "LangDAG documentation"
    }
  }
}
```

**Client → Server (tool result):**
```json
{
  "type": "tool_result",
  "data": {
    "call_id": "call_123",
    "result": "Search results..."
  }
}
```

---

## Error Responses

All errors follow this format:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Conversation not found",
    "details": {}
  }
}
```

### Error Codes

| Code | HTTP Status | Description |
|:-----|:------------|:------------|
| `invalid_request` | 400 | Malformed request |
| `unauthorized` | 401 | Invalid API key |
| `not_found` | 404 | Resource not found |
| `rate_limited` | 429 | Too many requests |
| `provider_error` | 502 | LLM provider error |
| `internal_error` | 500 | Server error |
