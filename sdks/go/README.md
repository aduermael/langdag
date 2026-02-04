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
    // Create a client
    client := langdag.NewClient("http://localhost:8080",
        langdag.WithAPIKey("your-api-key"),
    )

    ctx := context.Background()

    // Start a new chat
    resp, err := client.Chat(ctx, &langdag.NewChatRequest{
        Message: "Hello, how are you?",
        Model:   "claude-sonnet-4-20250514",
    }, nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Response: %s\n", resp.Content)

    // Continue the conversation
    resp, err = client.ContinueChat(ctx, resp.DAGID, &langdag.ContinueChatRequest{
        Message: "Tell me a joke",
    }, nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Response: %s\n", resp.Content)
}
```

## Streaming

The SDK supports Server-Sent Events (SSE) for real-time streaming responses:

```go
err := client.ChatStream(ctx, &langdag.NewChatRequest{
    Message: "Write a poem about coding",
}, func(event langdag.SSEEvent) error {
    switch event.Type {
    case langdag.SSEEventStart:
        fmt.Printf("Started conversation: %s\n", event.DAGID)
    case langdag.SSEEventDelta:
        fmt.Print(event.Content)
    case langdag.SSEEventDone:
        fmt.Printf("\nCompleted: node %s\n", event.NodeID)
    case langdag.SSEEventError:
        return fmt.Errorf("stream error: %s", event.Error)
    }
    return nil
})
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

### DAG Operations

```go
// List all DAGs
dags, err := client.ListDAGs(ctx)

// Get a specific DAG (supports UUID prefix)
dag, err := client.GetDAG(ctx, "abc123")

// Delete a DAG
resp, err := client.DeleteDAG(ctx, "abc123")
```

### Chat Operations

```go
// Start a new conversation
resp, err := client.Chat(ctx, &langdag.NewChatRequest{
    Message:      "Hello!",
    Model:        "claude-sonnet-4-20250514",
    SystemPrompt: "You are a helpful assistant.",
}, nil)

// Continue a conversation
resp, err := client.ContinueChat(ctx, dagID, &langdag.ContinueChatRequest{
    Message: "Tell me more",
}, nil)

// Fork a conversation from a specific node
resp, err := client.ForkChat(ctx, dagID, &langdag.ForkChatRequest{
    NodeID:  nodeID,
    Message: "What if we tried a different approach?",
}, nil)
```

### Workflow Operations

```go
// List workflows
workflows, err := client.ListWorkflows(ctx)

// Create a workflow
workflow, err := client.CreateWorkflow(ctx, &langdag.CreateWorkflowRequest{
    Name:        "my-workflow",
    Description: "A sample workflow",
    Nodes: []langdag.WorkflowNode{
        {ID: "input", Type: langdag.WorkflowNodeTypeInput},
        {ID: "llm", Type: langdag.WorkflowNodeTypeLLM, Prompt: "Process: {{input}}"},
        {ID: "output", Type: langdag.WorkflowNodeTypeOutput},
    },
    Edges: []langdag.WorkflowEdge{
        {From: "input", To: "llm"},
        {From: "llm", To: "output"},
    },
})

// Run a workflow
result, err := client.RunWorkflow(ctx, "my-workflow", &langdag.RunWorkflowRequest{
    Input: map[string]interface{}{
        "text": "Hello, world!",
    },
})
```

## Error Handling

The SDK provides typed errors for better error handling:

```go
resp, err := client.GetDAG(ctx, "nonexistent")
if err != nil {
    var apiErr *langdag.APIError
    if errors.As(err, &apiErr) {
        if apiErr.IsNotFound() {
            fmt.Println("DAG not found")
        } else if apiErr.IsUnauthorized() {
            fmt.Println("Invalid API key")
        }
    }
}
```

## License

MIT License - see [LICENSE](../../LICENSE) for details.
