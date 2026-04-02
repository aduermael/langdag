# LangDAG SSE Streaming Format

Server-Sent Events (SSE) format used by the LangDAG API for streaming responses.

## Wire Format

Each event consists of an `event:` line, one or more `data:` lines, and a blank-line terminator (`\n\n`):

```
event: <type>\n
data: <payload>\n
\n
```

Multi-line payloads use one `data:` prefix per line (per SSE spec, RFC 8895):

```
event: error\n
data: line one\n
data: line two\n
\n
```

Clients MUST join multiple `data:` lines with `\n` before processing.

## Event Types

### `start`

Emitted once at the beginning of a stream. Payload is `{}`.

```
event: start
data: {}
```

### `delta`

Emitted zero or more times with content chunks. Payload is JSON: `{"content":"<text>"}`.

```
event: delta
data: {"content":"Hello "}
```

### `done`

Emitted once when the response is complete. Payload is JSON: `{"node_id":"<id>"}`.

```
event: done
data: {"node_id":"abc123"}
```

### `error`

Emitted when an error occurs. Payload is **plain text** (not JSON). May contain newlines
(each line gets its own `data:` prefix on the wire).

```
event: error
data: provider crashed: connection reset
```

Multi-line error example on the wire:

```
event: error
data: line one
data: line two
```

After joining `data:` lines, the error message is: `"line one\nline two"`.

## Event Sequences

**Normal completion:**  `start` → `delta`* → `done`

**Error mid-stream:**   `start` → `delta`* → `error`

**Immediate error:**    `error`

**Abnormal close:**     `start` → `delta`* → (connection drops, no `done` or `error`)

## SDK Error Mapping

| Scenario | Go SDK | Python SDK | TypeScript SDK |
|----------|--------|------------|----------------|
| Error event | `StreamError{Message}` via `Err()` | `SSEEvent(ERROR, {"message": msg})` | `SSEEvent{type:'error', error: msg}` |
| No done event | `StreamError` from `Node()` | Iteration completes, no node_id | `SSEParseError` from `node()` |
| Content after error | `Content()` returns partial | Manual accumulation from deltas | `stream.content` returns partial |

## Malformed Data Handling (SDK-Specific)

Malformed JSON in `delta` or `done` payloads:

| SDK | Behavior | Rationale |
|-----|----------|-----------|
| Go | Silent: event emitted with empty field, stream continues | Resilient — one bad chunk shouldn't kill the stream |
| Python | Fallback: data wrapped as `{"message": raw_text}` | Consistent access pattern via `.data` dict |
| TypeScript | Strict: `SSEParseError` thrown, stops iteration | Fail-fast — catches protocol bugs early |

This divergence is intentional. Each approach serves its language ecosystem's conventions. The server always sends valid JSON for `delta` and `done` events — malformed data indicates a bug or corruption, and the SDK's job is to surface it in the way most natural for its users.
