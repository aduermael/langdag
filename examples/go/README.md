# LangDAG Go SDK Example

This example demonstrates the LangDAG Go SDK with a coherent conversation workflow:

1. Start a chat asking about Go vs Rust
2. Continue the conversation with a follow-up question
3. Fork from an earlier node to explore an alternative direction
4. List all DAGs and display the branching structure

## Prerequisites

- Go 1.21 or later
- A running LangDAG server

## Running the Example

Start the LangDAG server:

```bash
langdag serve
```

In another terminal, run the example:

```bash
cd examples/go
go run main.go
```

## Configuration

Set environment variables to customize the connection:

```bash
# Server URL (default: http://localhost:8080)
export LANGDAG_URL=http://localhost:8080

# API key (if authentication is enabled)
export LANGDAG_API_KEY=your-api-key
```

## What the Example Shows

- **Streaming responses**: Using `ChatStream`, `ContinueChatStream`, and `ForkChatStream`
- **SSE event handling**: Processing `start`, `delta`, `done`, and `error` events
- **DAG exploration**: Listing DAGs with `ListDAGs` and viewing details with `GetDAG`
- **Branching**: Creating alternative conversation paths with `ForkChatStream`
