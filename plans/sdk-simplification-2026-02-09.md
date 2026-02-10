# SDK Client API Simplification

Redesign the client SDK interface around **nodes** instead of chat verbs. The current API has 6 chat methods (`Chat`, `ChatStream`, `ContinueChat`, `ContinueChatStream`, `ForkChat`, `ForkChatStream`) that obscure the core abstraction: a DAG of nodes where any node can be extended with a new message.

## Design Philosophy

The DAG is the central concept. A conversation is just a DAG with user/assistant nodes. "Forking" and "continuing" are the same operation: **creating a new child node from any existing node**. The distinction between "continue" and "fork" is a server concern (continue = append to latest leaf, fork = branch from a specific node), but from the client's perspective it's always: "given this node, say something new."

## Proposed Go Client API

```go
// Create client
client := langdag.NewClient("http://localhost:8080")

// Start a new conversation (returns a Node)
node, err := client.Chat(ctx, "What is LangDAG?", langdag.WithSystem("You are helpful."))
fmt.Println(node.Content) // assistant response

// Continue from any node (this is the core operation)
node2, err := node.Reply(ctx, "Tell me more")
fmt.Println(node2.Content)

// "Fork" is just replying to an earlier node
alt, err := node.Reply(ctx, "What about the architecture?")
// node now has two children: node2 and alt

// Streaming variant
stream, err := node.ReplyStream(ctx, "Tell me more")
for event := range stream.Events() {
    fmt.Print(event.Content)
}
resultNode, err := stream.Node() // get final node after stream completes

// Get the full DAG
dag, err := client.GetDAG(ctx, dagID)
for _, n := range dag.Nodes {
    fmt.Printf("[%s] %s: %s\n", n.ID[:8], n.Type, n.Content[:50])
}

// Any retrieved node can be replied to
someNode := dag.Nodes[2]
reply, err := someNode.Reply(ctx, "Expand on this point")

// List all DAGs
dags, err := client.ListDAGs(ctx)
```

### Key Changes

| Current | Proposed | Why |
|---------|----------|-----|
| `client.ChatStream(ctx, req, handler)` | `client.Chat(ctx, msg, opts...)` | Start a conversation, get a Node back |
| `client.ContinueChatStream(ctx, dagID, req, handler)` | `node.Reply(ctx, msg)` | Continue from any node |
| `client.ForkChatStream(ctx, dagID, req, handler)` | `node.Reply(ctx, msg)` | Same as continue - fork is just replying to a non-leaf |
| SSE callback handler | `stream.Events()` channel or `range` | More idiomatic Go |
| `NewChatRequest{Message, SystemPrompt}` | `client.Chat(ctx, msg, WithSystem(...))` | Functional options |
| `ForkChatRequest{NodeID, Message}` | `node.Reply(ctx, msg)` | Node already knows its ID and DAG |

### Core Types

```go
type Node struct {
    ID        string
    DAGID     string
    ParentID  string
    Type      string    // "user", "assistant", etc.
    Content   string
    Sequence  int
    TokensIn  int
    TokensOut int
    CreatedAt time.Time

    client    *Client   // unexported, enables Reply()
}

type Stream struct { /* ... */ }
func (s *Stream) Events() <-chan SSEEvent  // channel-based iteration
func (s *Stream) Node() (*Node, error)    // final node after stream ends

type DAG struct {
    ID        string
    Title     string
    Status    string
    Nodes     []Node
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### Client Methods

```go
// Minimal top-level API
func (c *Client) Chat(ctx, message, ...ChatOption) (*Node, error)
func (c *Client) ChatStream(ctx, message, ...ChatOption) (*Stream, error)
func (c *Client) GetDAG(ctx, id) (*DAG, error)
func (c *Client) ListDAGs(ctx) ([]DAG, error)
func (c *Client) DeleteDAG(ctx, id) error
func (c *Client) Health(ctx) error

// Node methods
func (n *Node) Reply(ctx, message) (*Node, error)
func (n *Node) ReplyStream(ctx, message) (*Stream, error)
func (n *Node) Children() []Node  // if DAG was fetched with nodes
```

---

## Phase 1: Design & Validate Go SDK API

Finalize types and method signatures in the Go SDK.

- [ ] 1a: Define new `Node`, `Stream`, `DAG` types and `Client` method signatures
- [ ] 1b: Update Go SDK implementation (client.go, types, streaming)
- [ ] 1c: Update Go SDK unit tests

---

## Phase 2: Update Go Example

- [ ] 2a: Rewrite `examples/go/main.go` to use the new node-centric API

---

## Phase 3: Align Python SDK

Port the same node-centric pattern to Python.

- [ ] 3a: Redesign Python SDK with `Node.reply()` / `Node.reply_stream()` pattern
- [ ] 3b: Update Python SDK unit tests
- [ ] 3c: Rewrite `examples/python/example.py`

---

## Phase 4: Align TypeScript SDK

Port the same node-centric pattern to TypeScript.

- [ ] 4a: Redesign TypeScript SDK with `node.reply()` / `node.replyStream()` pattern
- [ ] 4b: Update TypeScript SDK unit tests
- [ ] 4c: Rewrite `examples/typescript/example.ts`

---

## Phase 5: Update Website & Docs

- [ ] 5a: Update website code examples to use new API
- [ ] 5b: Update README SDK sections
- [ ] 5c: Update OpenAPI spec if any endpoints changed

---

## Notes

- **No server API changes needed** - the existing REST endpoints (`POST /chat`, `POST /chat/{id}`, `POST /chat/{id}/fork`) stay the same. The simplification is purely client-side.
- The `Node` struct holds an unexported `client` reference, enabling `node.Reply()` without the user having to pass the client around.
- Workflow methods (`ListWorkflows`, `CreateWorkflow`, `RunWorkflow`) are kept as-is on the client for now since they're a separate concern.
