# LangDAG Go SDK Example

This example demonstrates the LangDAG Go SDK with a coherent conversation workflow:

1. Start a conversation with `client.Prompt()`
2. Stream a response with `client.PromptStream()`
3. Continue from a node with `node.Prompt()`
4. Branch from an earlier node to explore an alternative direction
5. List root nodes and display the tree structure

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

- **Prompt/PromptStream**: Starting conversations with `client.Prompt()` and `client.PromptStream()`
- **Node continuation**: Extending conversations with `node.Prompt()`
- **Branching**: Creating alternative paths by prompting from earlier nodes
- **Tree exploration**: Listing root nodes with `client.ListRoots()` and viewing trees with `client.GetTree()`
