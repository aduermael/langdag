# Fallback Providers, A/B Testing & Routing Strategy

**Goal:** Replace single-provider selection with a multi-provider routing system that supports fallback chains, weighted A/B testing, per-provider retry configs, and automatic API key availability filtering. This is critical for migrating off LangGraph while preserving fallback provider behavior.

## Design Overview

A new `Router` wraps multiple `Provider` instances and implements the `Provider` interface itself. It is configured via YAML/env vars and supports:

- **Weighted routing**: e.g. Anthropic 90%, Vertex 10% — chosen probabilistically per request
- **Fallback chains**: if the selected provider fails (after its own retries), try the next one
- **Per-provider retry**: each provider gets its own retry config (or inherits global defaults)
- **API key filtering**: providers without valid credentials are automatically excluded at startup
- **Transparent**: the Router satisfies the `Provider` interface, so the conversation manager doesn't change

### Config Example (YAML)

```yaml
providers:
  routing:
    - provider: anthropic
      weight: 90        # 90% of traffic
      retry:
        max_retries: 3
        base_delay: 1s
        max_delay: 30s
    - provider: openai
      weight: 10        # 10% of traffic
      retry:
        max_retries: 2
        base_delay: 500ms
        max_delay: 10s
  fallback_order:       # on failure, try these in order
    - anthropic
    - openai
    - gemini
```

When `providers.routing` is not set, behavior is identical to today (single `providers.default` provider).

### Env Var Overrides

- `LANGDAG_ROUTING`: JSON array override, e.g. `[{"provider":"anthropic","weight":90},{"provider":"openai","weight":10}]`
- `LANGDAG_FALLBACK_ORDER`: comma-separated, e.g. `anthropic,openai,gemini`
- Existing per-provider API key env vars (`ANTHROPIC_API_KEY`, etc.) still work — providers without keys are silently dropped from routing/fallback

---

## Phase 1: Provider Router Core

- [ ] 1a: Create `internal/provider/router.go` — `Router` struct implementing `Provider` interface with weighted random selection, fallback chain on failure, and API key availability filtering at construction time
- [ ] 1b: Create `internal/provider/router_test.go` — unit tests for weighted selection distribution, fallback on error, skip unavailable providers, all-fail case

## Phase 2: Per-Provider Retry Config

- [ ] 2a: Extend config types — add `RoutingEntry` struct (provider name, weight, per-provider retry config) and `[]RoutingEntry` + `FallbackOrder []string` fields to `ProvidersConfig`
- [ ] 2b: Wire per-provider retry — each provider in the router gets its own `WithRetry` wrapper based on its entry's retry config (falling back to global retry config if not specified)

## Phase 3: Config & Factory Integration

- [ ] 3a: Update `config.go` — add viper bindings for `providers.routing` list and `providers.fallback_order`, plus `LANGDAG_ROUTING` and `LANGDAG_FALLBACK_ORDER` env var overrides
- [ ] 3b: Update `createProvider()` in `server.go` — when `providers.routing` is present, build all available providers, wrap each with its retry config, construct the Router; otherwise keep current single-provider behavior

## Phase 4: Observability & Metadata

- [ ] 4a: Add provider name to completion response metadata — extend `CompletionResponse` (or `StreamEvent`) so the caller knows which provider actually served the request (useful for A/B analysis)
- [ ] 4b: Log routing decisions — log which provider was selected, whether fallback was triggered, and which provider ultimately succeeded

## Phase 5: Documentation & Tests

- [ ] 5a: Add integration test with mock providers — test full routing + fallback + retry flow through the server using mock providers with configurable failure modes
- [ ] 5b: Add example config in `examples/` — sample `config.yaml` showing routing, fallback, and per-provider retry setup
