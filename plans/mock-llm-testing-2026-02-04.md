# Mock LLM Server & Testing Infrastructure

This plan implements a mock LLM API server in Go for testing without consuming AI tokens, and establishes a comprehensive test suite for all SDKs with GitHub Actions CI.

---

## Phase 1: Mock LLM Server (Go Tool)

Create a standalone mock LLM server that mimics the Anthropic API for testing purposes.

- [x] 1a: Create `tools/mockllm/` Go module with HTTP server supporting both streaming and non-streaming modes
- [x] 1b: Implement Anthropic-compatible `/v1/messages` endpoint with configurable response behavior (random text, fixed responses, delays)
- [x] 1c: Implement SSE streaming that mimics real token-by-token output with configurable chunk sizes and delays
- [x] 1d: Add response modes: `random` (random lorem ipsum), `echo` (echo user message), `fixed` (configurable response), `error` (simulate errors)
- [x] 1e: Add CLI flags for port, mode, delay, and response configuration
- [ ] 1f: Add Makefile target to build and run mockllm

---

## Phase 2: LangDAG Server Test Configuration

Enable LangDAG server to use the mock LLM provider.

- [ ] 2a: Create mock provider in `internal/provider/mock/` implementing the Provider interface
- [ ] 2b: Add provider selection via environment variable or config (`LANGDAG_PROVIDER=mock`)
- [ ] 2c: Update config to support mock provider settings (response mode, delay, etc.)

---

## Phase 3: Go SDK Tests

Unit tests and E2E test for the Go SDK.

- [ ] 3a: Add unit tests for `sdks/go/` covering client initialization, request building, error handling
- [ ] 3b: Add unit tests for SSE parsing and event handling
- [ ] 3c: Add E2E test that spins up mock server + langdag server and tests full chat flow (streaming & non-streaming)

---

## Phase 4: Python SDK Tests

Unit tests and E2E test for the Python SDK.

- [ ] 4a: Add unit tests for sync client using pytest-httpx mocking
- [ ] 4b: Add unit tests for async client using pytest-httpx async mocking
- [ ] 4c: Add E2E test that connects to langdag server with mock provider

---

## Phase 5: TypeScript SDK Tests

Unit tests and E2E test for the TypeScript SDK.

- [ ] 5a: Set up Jest or Vitest testing framework in `sdks/typescript/`
- [ ] 5b: Add unit tests for client methods and error handling
- [ ] 5c: Add unit tests for SSE parsing
- [ ] 5d: Add E2E test that connects to langdag server with mock provider

---

## Phase 6: Test Infrastructure & Scripts

Create test orchestration scripts.

- [ ] 6a: Create `scripts/test-e2e.sh` that starts mock provider, langdag server, runs all SDK E2E tests, and cleans up
- [ ] 6b: Add Makefile targets: `test`, `test-unit`, `test-e2e`, `test-go`, `test-python`, `test-typescript`
- [ ] 6c: Create Docker Compose file for test environment (mockllm + langdag server)

---

## Phase 7: GitHub Actions CI

Set up automated testing on each commit.

- [ ] 7a: Create `.github/workflows/test.yml` with jobs for each SDK's unit tests
- [ ] 7b: Add E2E test job that builds mockllm, starts services, runs E2E tests
- [ ] 7c: Add matrix testing for Go versions (1.22, 1.23), Python (3.10, 3.11, 3.12), Node.js (18, 20, 22)
- [ ] 7d: Add test status badge to README

---

## Summary

| Phase | Focus |
|-------|-------|
| 1 | Mock LLM server tool (Go) |
| 2 | LangDAG mock provider integration |
| 3 | Go SDK unit + E2E tests |
| 4 | Python SDK unit + E2E tests |
| 5 | TypeScript SDK unit + E2E tests |
| 6 | Test scripts and orchestration |
| 7 | GitHub Actions CI pipeline |

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Test Environment                     │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ SDK Test │───▶│   LangDAG    │───▶│    Mock      │  │
│  │ (Go/Py/  │    │   Server     │    │   Provider   │  │
│  │    TS)   │◀───│  (REST API)  │◀───│  (in-proc)   │  │
│  └──────────┘    └──────────────┘    └──────────────┘  │
│                                                          │
│  OR (standalone testing):                                │
│                                                          │
│  ┌──────────┐    ┌──────────────┐                       │
│  │ Anthropic│───▶│   MockLLM    │  (standalone tool)   │
│  │ SDK Test │◀───│   Server     │                       │
│  └──────────┘    └──────────────┘                       │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

The mock provider is integrated directly into LangDAG server, making E2E tests fast and deterministic without needing to spin up a separate mock LLM service.
