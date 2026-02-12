# LangDAG Go SDK

Official Go client library for the [LangDAG](https://github.com/langdag/langdag) REST API.

## Installation

```bash
go get github.com/langdag/langdag-go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    langdag "github.com/langdag/langdag-go"
)

func main() {
    client := langdag.NewClient("http://localhost:8080")
    ctx := context.Background()

    // Start a new conversation
    node, err := client.Prompt(ctx, "Hello, how are you?")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Response: %s\n", node.Content)

    // Continue from any node
    node2, err := node.Prompt(ctx, "Tell me a joke")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Response: %s\n", node2.Content)
}
```

## Streaming

The SDK supports Server-Sent Events (SSE) for real-time streaming responses:

```go
stream, err := client.PromptStream(ctx, "Write a poem about coding")
if err != nil {
    log.Fatal(err)
}

for event := range stream.Events() {
    fmt.Print(event.Content)
}

// Get the final node after streaming completes
result, err := stream.Node()
fmt.Printf("\nNode ID: %s\n", result.ID)
```

## API Reference

### Client Creation

```go
// Basic client
client := langdag.NewClient("http://localhost:8080")

// With API key authentication
client := langdag.NewClient("http://localhost:8080",
    langdag.WithAPIKey("your-api-key"),
)

// With bearer token authentication
client := langdag.NewClient("http://localhost:8080",
    langdag.WithBearerToken("your-token"),
)

// With custom HTTP client
client := langdag.NewClient("http://localhost:8080",
    langdag.WithHTTPClient(customHTTPClient),
)

// With custom timeout
client := langdag.NewClient("http://localhost:8080",
    langdag.WithTimeout(60 * time.Second),
)
```

### Prompt Operations

```go
// Start a new conversation (creates root node)
node, err := client.Prompt(ctx, "Hello!")
node, err := client.Prompt(ctx, "Hello!",
    langdag.WithSystem("You are helpful."),
    langdag.WithModel("claude-sonnet-4-20250514"),
)

// Continue from any node
node2, err := node.Prompt(ctx, "Tell me more")

// Branch from an earlier node
alt, err := node.Prompt(ctx, "Different angle")

// Stream a new conversation
stream, err := client.PromptStream(ctx, "Tell me a story")
for event := range stream.Events() {
    fmt.Print(event.Content)
}
result, err := stream.Node()

// Stream from an existing node
stream, err := node.PromptStream(ctx, "Explain in detail")
```

### Node Operations

```go
// Get a node by ID
node, err := client.GetNode(ctx, "abc123")

// Get a full tree from a node
tree, err := client.GetTree(ctx, "abc123")
for _, n := range tree.Nodes {
    fmt.Printf("[%s] %s\n", n.Type, n.Content)
}

// List root nodes (conversations)
roots, err := client.ListRoots(ctx)

// Delete a node and its subtree
err := client.DeleteNode(ctx, "abc123")
```

### Workflow Operations

```go
// List workflows
workflows, err := client.ListWorkflows(ctx)

// Create a workflow
workflow, err := client.CreateWorkflow(ctx, &langdag.CreateWorkflowRequest{...})

// Run a workflow
result, err := client.RunWorkflow(ctx, "my-workflow", &langdag.RunWorkflowRequest{...})
```

## Error Handling

The SDK provides typed errors for better error handling:

```go
node, err := client.GetNode(ctx, "nonexistent")
if err != nil {
    var apiErr *langdag.APIError
    if errors.As(err, &apiErr) {
        if apiErr.IsNotFound() {
            fmt.Println("Node not found")
        } else if apiErr.IsUnauthorized() {
            fmt.Println("Invalid API key")
        }
    }
}
```

## License

MIT License - see [LICENSE](../../LICENSE) for details.
