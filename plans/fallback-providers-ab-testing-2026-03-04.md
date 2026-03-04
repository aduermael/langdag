# Fallback Providers, A/B Testing & Routing Strategy

**Goal:** Replace single-provider selection with a multi-provider routing system that supports fallback chains, weighted A/B testing, per-provider retry configs, and automatic credential filtering. Also restructure providers into protocol groups so adding new endpoints (Vertex, Bedrock, Azure) is trivial.

## Design Overview

### Provider Group Architecture

Providers are organized by **API protocol** — variants within a group share all message conversion, streaming, and response parsing code. Only auth, base URL, and model list differ per variant.

```
internal/provider/
├── anthropic/
│   ├── protocol.go          # Shared: message/tool conversion, response parsing, streaming
│   ├── direct.go            # Direct Anthropic API (SDK-based, x-api-key auth)
│   ├── bedrock.go           # AWS Bedrock (HTTP + AWS SigV4 auth)
│   └── vertex.go            # Vertex AI (HTTP + Google OAuth)
├── openai/
│   ├── protocol.go          # Shared: message/tool conversion, SSE parsing
│   ├── direct.go            # Direct OpenAI API (Bearer token auth)
│   └── azure.go             # Azure OpenAI (api-key header, different URL scheme)
├── gemini/
│   ├── protocol.go          # Shared: message/tool conversion, SSE snapshot diffing
│   ├── direct.go            # Direct Gemini API (query param auth)
│   └── vertex.go            # Vertex AI Gemini (Google OAuth, different base URL)
├── mock/
│   └── mock.go              # Unchanged
├── provider.go              # Provider interface
├── retry.go                 # Retry wrapper (unchanged)
└── router.go                # NEW: weighted routing + fallback
```

Each variant is a distinct provider name in config: `anthropic`, `anthropic-vertex`, `anthropic-bedrock`, `openai`, `openai-azure`, `gemini`, `gemini-vertex`, etc.

### Router

A new `Router` wraps multiple `Provider` instances and implements the `Provider` interface itself:

- **Weighted routing**: e.g. Anthropic 90%, Vertex 10% — chosen probabilistically per request
- **Fallback chains**: if the selected provider fails (after its own retries), try the next one in fallback order
- **Per-provider retry**: each provider gets its own retry config (or inherits global defaults)
- **Credential filtering**: providers without valid credentials are automatically excluded at startup
- **Transparent**: satisfies `Provider` interface, conversation manager doesn't change

### Config Example (YAML)

```yaml
providers:
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
  anthropic-vertex:
    project_id: my-gcp-project
    region: us-east5
  openai:
    api_key: ${OPENAI_API_KEY}

  routing:
    - provider: anthropic
      weight: 80
      retry:
        max_retries: 3
        base_delay: 1s
        max_delay: 30s
    - provider: anthropic-vertex
      weight: 20
      retry:
        max_retries: 2
        base_delay: 500ms
        max_delay: 10s

  fallback_order:
    - anthropic
    - anthropic-vertex
    - openai
```

When `providers.routing` is not set, behavior is identical to today (single `providers.default` provider).

### Env Var Overrides

- `LANGDAG_ROUTING`: JSON array, e.g. `[{"provider":"anthropic","weight":80},{"provider":"anthropic-vertex","weight":20}]`
- `LANGDAG_FALLBACK_ORDER`: comma-separated, e.g. `anthropic,anthropic-vertex,openai`
- Per-variant env vars: `ANTHROPIC_API_KEY`, `VERTEX_PROJECT_ID`, `VERTEX_REGION`, `AWS_REGION`, `OPENAI_API_KEY`, `AZURE_OPENAI_API_KEY`, `AZURE_OPENAI_ENDPOINT`, etc.
- Providers without valid credentials are silently dropped from routing/fallback

---

## Phase 1: Refactor Anthropic Provider into Protocol + Variants

Extract shared protocol code from the current monolithic `anthropic.go`, then add Vertex and Bedrock variants.

- [x] 1a: Split `anthropic.go` into `protocol.go` (message conversion, tool conversion, response parsing, stream event handling) and `direct.go` (client construction, auth, Complete/Stream methods that call protocol helpers)
- [x] 1b: Add `vertex.go` — Vertex AI variant using HTTP client with Google OAuth, same protocol helpers
- [x] 1c: Add `bedrock.go` — AWS Bedrock variant using HTTP client with SigV4 auth, same protocol helpers
- [x] 1d: Tests for new variants (can be unit tests with HTTP mocks for auth/endpoint logic)

## Phase 2: Refactor OpenAI Provider into Protocol + Variants

- [x] 2a: Split `openai.go` into `protocol.go` (message/tool conversion, SSE parsing, response mapping) and `direct.go` (already supports base_url, just restructure)
- [ ] 2b: Add `azure.go` — Azure OpenAI variant (different URL scheme: `{endpoint}/openai/deployments/{model}/chat/completions?api-version=...`, `api-key` header)
- [ ] 2c: Tests for Azure variant

## Phase 3: Refactor Gemini Provider into Protocol + Variants

- [ ] 3a: Split `gemini.go` into `protocol.go` and `direct.go`
- [ ] 3b: Add `vertex.go` — Vertex AI Gemini variant (different base URL, Google OAuth instead of API key)
- [ ] 3c: Tests for Vertex Gemini variant

## Phase 4: Provider Router Core

- [ ] 4a: Create `internal/provider/router.go` — `Router` struct implementing `Provider` interface with weighted random selection, fallback chain on failure, and credential availability filtering at construction time
- [ ] 4b: Create `internal/provider/router_test.go` — unit tests for weighted selection distribution, fallback on error, skip unavailable providers, all-fail case

## Phase 5: Config, Factory & Per-Provider Retry

- [ ] 5a: Extend config types — add per-variant config structs (Vertex: project_id, region; Bedrock: region; Azure: endpoint, api_version), `RoutingEntry` (provider name, weight, retry config), routing list, and fallback_order to `ProvidersConfig`
- [ ] 5b: Update `config.go` — add viper bindings for new provider variants, routing list, fallback_order, and env var overrides
- [ ] 5c: Update `createProvider()` in `server.go` — provider registry mapping names to factory functions; when routing config is present, build all available providers, wrap each with per-provider retry (falling back to global), construct Router; otherwise keep single-provider behavior

## Phase 6: Observability & Metadata

- [ ] 6a: Add provider name to completion response metadata — extend `CompletionResponse` or `StreamEvent` so the caller knows which provider actually served the request (useful for A/B analysis)
- [ ] 6b: Log routing decisions — log which provider was selected, whether fallback was triggered, and which provider ultimately succeeded

## Phase 7: Documentation & Integration Tests

- [ ] 7a: Add integration test with mock providers — test full routing + fallback + retry flow using mock providers with configurable failure modes
- [ ] 7b: Add example config in `examples/` — sample `config.yaml` showing multi-provider routing, fallback, and per-provider retry setup
