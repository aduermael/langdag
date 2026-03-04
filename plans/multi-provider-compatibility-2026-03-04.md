# Multi-Provider Compatibility & Token Tracking

Make langdag work with any LLM provider (OpenAI, Grok, Gemini, Mistral, etc.) with a unified token tracking model. Also documents the LangGraph feature gap for future reference.

## Current State

**What's already generic:**
- Provider interface (`Complete`, `Stream`, `Name`, `Models`) is clean
- Config struct has `OpenAI` provider config (unused)
- `CompletionRequest` separates `System` from `Messages` (correct for all providers)
- `ContentBlock` struct supports text, tool_use, tool_result
- Mock provider proves the interface works generically

**What's Anthropic-shaped:**
- Only Anthropic + Mock providers implemented
- `Usage` struct only has `InputTokens` + `OutputTokens` — no cache or reasoning tokens
- DB schema only stores `tokens_in` + `tokens_out`
- Stream events hardcoded to Anthropic event types
- Provider factory defaults to Anthropic
- SDKs and API responses only expose 2 token fields
- No image/document content block fields

## Unified Token Usage Model

All providers report input and output tokens. Some also report cache and reasoning tokens:

| Field | OpenAI | Anthropic | xAI | Gemini | Mistral |
|---|---|---|---|---|---|
| Input tokens | `prompt_tokens` | `input_tokens` | `prompt_tokens` | `promptTokenCount` | `prompt_tokens` |
| Output tokens | `completion_tokens` | `output_tokens` | `completion_tokens` | `candidatesTokenCount` | `completion_tokens` |
| Cached input | `prompt_tokens_details.cached_tokens` | `cache_read_input_tokens` | same as OpenAI | `cachedContentTokenCount` | — |
| Cache write | — | `cache_creation_input_tokens` | — | — | — |
| Reasoning | `completion_tokens_details.reasoning_tokens` | (in output_tokens) | same as OpenAI | `thoughtsTokenCount` | — |

**New `Usage` struct:**
```go
type Usage struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
    ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
}
```

Each provider adapter maps its native fields into this unified struct.

## Provider API Families

Three wire-format families exist:

1. **OpenAI-compatible** (OpenAI, xAI/Grok, Mistral): Same message format, roles, tool calling, SSE streaming. A single adapter handles all three (just different base URLs and API keys).
2. **Anthropic**: Unique content-block format, top-level `system` param, different streaming events.
3. **Google Gemini**: Parts-based format, `user`/`model` roles, `FunctionCall`/`FunctionResponse` parts, different streaming model.

Key adapter responsibilities per family:
- **System prompt**: OpenAI-family prepends as `developer`/`system` role message; Anthropic uses top-level param; Gemini uses `system_instruction`
- **Tool calls**: OpenAI-family uses `tool_calls` array on assistant message; Anthropic uses `tool_use` content blocks; Gemini uses `functionCall` parts
- **Streaming**: OpenAI-family sends `chat.completion.chunk` deltas terminated by `[DONE]`; Anthropic sends `content_block_delta` events; Gemini sends full response snapshots

---

## Phase 1: Extend Token Usage (Backend)

Extend the `Usage` struct, database schema, executor, and API responses to support all token types.

- [x] 1a: Add `CacheReadInputTokens`, `CacheCreationInputTokens`, `ReasoningTokens` to `Usage` struct in `pkg/types/types.go`
- [x] 1b: Add matching fields to `Node` struct (`tokens_cache_read`, `tokens_cache_creation`, `tokens_reasoning`)
- [x] 1c: Add DB migration for new columns in `internal/storage/sqlite/migrations.go`
- [x] 1d: Update SQLite storage implementation to read/write new token fields
- [x] 1e: Update executor and conversation manager to propagate all token fields from `Usage` to `Node`
- [x] 1f: Update Anthropic provider to populate `CacheReadInputTokens` and `CacheCreationInputTokens` from Anthropic responses
- [x] 1g: Update API response structs to include new token fields
- [x] 1h: Update mock provider to set cache/reasoning token fields for testing

---

## Phase 2: OpenAI-Compatible Provider

Implement an OpenAI-compatible provider that works with OpenAI, xAI/Grok, and Mistral (all share the same wire format).

- [x] 2a: Create `internal/provider/openai/openai.go` implementing the `Provider` interface — `Complete` and `Stream` methods using the OpenAI chat completions API
- [x] 2b: Implement message conversion: unified `ContentBlock` to/from OpenAI message format (handle system-as-role, tool_calls array, tool role messages)
- [x] 2c: Implement token usage mapping: `prompt_tokens` → `InputTokens`, `completion_tokens` → `OutputTokens`, extract `cached_tokens` and `reasoning_tokens` from `_details` objects
- [x] 2d: Implement streaming: parse OpenAI SSE chunks (`chat.completion.chunk` with `delta` field), emit unified `StreamEvent` types
- [x] 2e: Support configurable `BaseURL` so the same provider works for xAI (`api.x.ai/v1`) and Mistral (`api.mistral.ai/v1`)
- [x] 2f: Wire into provider factory in `server.go` — support `LANGDAG_PROVIDER=openai` with `OPENAI_API_KEY`, `OPENAI_BASE_URL` env vars
- [x] 2g: Add OpenAI provider to config: `LANGDAG_OPENAI_API_KEY`, `LANGDAG_OPENAI_BASE_URL`
- [x] 2h: Add unit tests for message conversion, token mapping, and streaming parsing

---

## Phase 3: Gemini Provider

Implement Google Gemini provider.

- [x] 3a: Create `internal/provider/gemini/gemini.go` implementing `Provider` interface
- [x] 3b: Implement message conversion: unified format to/from Gemini's `contents`/`parts` format (role `assistant`→`model`, tool blocks→`functionCall`/`functionResponse` parts)
- [x] 3c: Implement token usage mapping: `promptTokenCount`→`InputTokens`, `candidatesTokenCount`→`OutputTokens`, `cachedContentTokenCount`→`CacheReadInputTokens`, `thoughtsTokenCount`→`ReasoningTokens`
- [x] 3d: Implement streaming: Gemini sends full response snapshots via SSE — diff consecutive snapshots to produce text deltas for unified `StreamEvent`
- [x] 3e: Wire into provider factory — `LANGDAG_PROVIDER=gemini` with `GEMINI_API_KEY`
- [x] 3f: Add unit tests

---

## Phase 4: SDK Token Field Updates

Update all three SDKs to expose the new token fields.

**Parallel Tasks: 4a, 4b, 4c**

- [x] 4a: Go SDK — add `CacheReadTokensIn`, `CacheCreationTokensIn`, `ReasoningTokens` to `Node` struct, update JSON parsing
- [x] 4b: Python SDK — add `cache_read_tokens_in`, `cache_creation_tokens_in`, `reasoning_tokens` to `Node` dataclass
- [x] 4c: TypeScript SDK — add `cacheReadTokensIn`, `cacheCreationTokensIn`, `reasoningTokens` to `NodeData` interface and `Node` class

---

## Phase 5: Content Block Improvements

Extend content blocks to support images and documents for multi-modal providers.

- [x] 5a: Add `MediaType`, `Data` (base64), and `URL` fields to `ContentBlock` in `pkg/types/types.go` for image/document support
- [x] 5b: Update Anthropic provider to handle image and document content blocks
- [x] 5c: Update OpenAI provider to convert image content blocks to `image_url` parts
- [x] 5d: Update Gemini provider to convert image blocks to `inlineData`/`fileData` parts

---

## LangGraph Feature Gap (Reference — Not Part of This Plan)

Features LangGraph has that langdag currently lacks, prioritized for future planning:

### Critical
- **Conditional edges / dynamic routing**: Branch node type exists but condition evaluation is unimplemented in the executor (returns nil)
- **Parallel node execution**: Executor runs nodes sequentially; independent branches could run concurrently with goroutines
- **Error handling / retries**: Any node failure fails the entire workflow; no retry policies
- **Typed state with reducers**: State is `map[string]json.RawMessage`; no typed schema or merge strategies

### Important
- **Checkpointing / durable execution**: Nodes saved to SQLite but intermediate workflow state not checkpointed; can't resume from failure
- **Human-in-the-loop**: No `interrupt`/`resume` mechanism for pausing workflows
- **Subgraphs / composition**: Workflows are flat; can't nest workflows as nodes
- **Cycles / iterative loops**: Explicitly forbidden by validator (DAG-only by design — may be intentional)

### Valuable
- **Multiple streaming modes**: Only token deltas; no state-update or debug streaming
- **Observability / tracing**: Basic metrics in SQLite; no OpenTelemetry integration
- **Node caching**: No cache layer for identical LLM inputs
- **Memory store**: No cross-workflow persistent storage with semantic search
