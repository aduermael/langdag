# SDK Test Strengthening Plan

Goal: Review and strengthen tests for all 3 SDKs (Go, Python, TypeScript), ensure tests run on every commit via CI, and add per-SDK badges to the README.

Guiding principle: **Never patch/workaround tests to make them pass.** Tests must reflect real SDK behavior and uncover real bugs. If a test fails, the SDK code gets fixed, not the test.

---

## Phase 1: Audit & fix existing unit tests

Run each SDK's unit tests as-is. Identify any failures and fix them in the SDK source code (not in the tests).

- [x] 1a: Run all existing unit tests across all 3 SDKs and document any failures
- [x] 1b: Fix any SDK bugs uncovered by existing tests (none found — all tests pass)

## Phase 2: Strengthen Go SDK unit tests

Add missing unit test coverage for the Go SDK.

- [ ] 2a: Add streaming unit tests — SSE error events, malformed JSON in delta/done, empty data fields, scanner errors
- [ ] 2b: Add `WithHTTPClient()` option test, combined options test, context cancellation test
- [ ] 2c: Add `parseError()` edge case tests — empty body, non-JSON body, unknown status codes (e.g. 502)

**Parallel Tasks: 2a, 2b, 2c**

## Phase 3: Strengthen Python SDK unit tests

Add missing unit test coverage for both sync and async Python clients.

- [ ] 3a: Add streaming unit tests for sync client — test `prompt(stream=True)` and `prompt_from(stream=True)` iteration, SSE error events during stream, stream with non-JSON error responses
- [ ] 3b: Add streaming unit tests for async client — currently has ZERO streaming tests; mirror sync streaming coverage
- [ ] 3c: Add edge case tests — `_handle_response` with non-JSON error bodies, `_parse_sse_stream` with unknown event types, multi-line data fields

**Parallel Tasks: 3a, 3b, 3c**

## Phase 4: Strengthen TypeScript SDK unit tests

Add missing unit test coverage for the TypeScript SDK.

- [ ] 4a: Add Stream class tests — `stream.node()` called multiple times, stream error propagation, stream with empty content deltas
- [ ] 4b: Add SSE edge cases — multi-line data fields, SSE comments (lines starting with `:`), event blocks with missing data field
- [ ] 4c: Add client edge case tests — response body null handling, non-JSON error responses, custom fetch option validation

**Parallel Tasks: 4a, 4b, 4c**

## Phase 5: Strengthen E2E tests (all SDKs)

Expand E2E tests to cover node operations more thoroughly, including error cases. (Workflow E2E tests are excluded since workflows are not yet fully stabilized.)

- [ ] 5a: Go E2E — add streaming continuation from node, error cases (get non-existent node, delete non-existent node), node ID prefix lookup
- [ ] 5b: Python E2E — add streaming continuation (sync + async), error cases, verify node field parsing (timestamps, token counts)
- [ ] 5c: TypeScript E2E — add error cases, verify node metadata fields, streaming edge cases

**Parallel Tasks: 5a, 5b, 5c**

## Phase 6: CI per-SDK badges

Split the CI workflow into separate per-SDK workflows so each gets its own badge, and add those badges to the README.

- [ ] 6a: Create `.github/workflows/test-go.yml` for Go SDK unit tests (matrix: Go 1.22, 1.23)
- [ ] 6b: Create `.github/workflows/test-python.yml` for Python SDK unit tests (matrix: Python 3.10, 3.11, 3.12)
- [ ] 6c: Create `.github/workflows/test-typescript.yml` for TypeScript SDK unit tests (matrix: Node 20, 22)
- [ ] 6d: Update `.github/workflows/test.yml` to keep E2E tests (depends on per-SDK workflows passing)
- [ ] 6e: Add per-SDK test badges to README.md (Go SDK, Python SDK, TypeScript SDK — each linking to their workflow)

**Parallel Tasks: 6a, 6b, 6c**
