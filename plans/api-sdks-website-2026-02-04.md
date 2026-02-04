# LangDAG API, SDKs & Documentation Overhaul

This plan restructures the project to lead with the CLI experience, implement a REST API with OpenAPI spec, create client SDKs in Python/Go/TypeScript, and update the website to showcase all three languages.

---

## Phase 1: README Restructure (CLI-First Experience)

Rewrite README to prioritize the interactive CLI journey before introducing the API and workflows.

- [x] 1a: Update README with CLI-first quick start (setup key, chat, list dags, show dag, continue/fork, show branches)
- [x] 1b: Move workflow examples to a secondary section after CLI basics
- [x] 1c: Add new section for "Use as API Server" after CLI and workflow sections

---

## Phase 2: HTTP API Server Implementation

Build the REST API server using the existing config infrastructure.

- [x] 2a: Create `internal/api/` package with HTTP server setup (using chi or stdlib)
- [x] 2b: Implement DAG endpoints: `GET /dags`, `GET /dags/{id}`, `DELETE /dags/{id}`
- [x] 2c: Implement Chat endpoints: `POST /chat` (new conversation), `POST /chat/{id}` (continue), `POST /chat/{id}/fork` (fork from node)
- [x] 2d: Implement Workflow endpoints: `GET /workflows`, `POST /workflows`, `POST /workflows/{id}/run`
- [x] 2e: Add streaming support via SSE for chat responses
- [x] 2f: Add `langdag serve` CLI command to start the API server
- [x] 2g: Create Dockerfile for containerized deployment

---

## Phase 3: OpenAPI Specification

Create formal API documentation.

- [x] 3a: Write OpenAPI 3.1 spec in `api/openapi.yaml`
- [x] 3b: Add request/response schemas for all endpoints
- [x] 3c: Document SSE streaming format
- [x] 3d: Add authentication section (API key header)

---

## Phase 4: Client SDKs

Generate and publish client libraries.

**Parallel Tasks: 4a, 4b, 4c**

- [x] 4a: Create Python SDK in `sdks/python/` (using httpx, async support)
- [x] 4b: Create Go SDK in `sdks/go/` (using net/http)
- [x] 4c: Create TypeScript SDK in `sdks/typescript/` (using fetch, typed responses)
- [x] 4d: Add SDK installation instructions to README

---

## Phase 5: Example Projects

Create example applications demonstrating SDK usage.

**Parallel Tasks: 5a, 5b, 5c**

- [x] 5a: Python example: `examples/python/` - agent interaction, DAG exploration, branching
- [x] 5b: Go example: `examples/go/` - same functionality
- [x] 5c: TypeScript example: `examples/typescript/` - same functionality

---

## Phase 6: Website Update

Update the landing page with multi-language code examples.

- [x] 6a: Add language toggle component (Python / Go / TypeScript)
- [x] 6b: Create code snippets showing SDK usage for each language
- [x] 6c: Update demo section to show: agent interaction, DAG exploration, branching
- [x] 6d: Add quick CLI example at the top (brief, to showcase SDKs are the main feature)
- [x] 6e: Link to SDK documentation and example projects

---

## Summary

| Phase | Focus |
|-------|-------|
| 1 | README restructure (CLI-first) |
| 2 | HTTP API server implementation |
| 3 | OpenAPI specification |
| 4 | Python, Go, TypeScript SDKs |
| 5 | Example projects in all 3 languages |
| 6 | Website with language toggle |
