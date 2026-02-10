# SDK Client API Simplification

Redesign the client SDK interface around **nodes** instead of chat verbs. The current API has 6 chat methods (`Chat`, `ChatStream`, `ContinueChat`, `ContinueChatStream`, `ForkChat`, `ForkChatStream`) that obscure the core abstraction: a DAG of nodes where any node can be extended with a new message.

## Design Philosophy

The DAG is the central concept. A conversation is just a DAG with user/assistant nodes. "Forking" and "continuing" are the same operation: **creating a new child node from any existing node**. The distinction between "continue" and "fork" is a server concern (continue = append to latest leaf, fork = branch from a specific node), but from the client's perspective it's always: "given this node, say something new."

The single verb for this is **`Prompt`** — it means the same thing whether called on a `Client` (new DAG) or a `Node` (extend existing DAG).

## Proposed Go Client API

```go
client := langdag.NewClient("http://localhost:8080")

// Start a new conversation (returns the assistant's response Node)
node, err := client.Prompt(ctx, "What is LangDAG?", langdag.WithSystem("You are helpful."))
fmt.Println(node.Content)

// Continue from any node — same method
node2, err := node.Prompt(ctx, "Tell me more")

// "Fork" is just prompting an earlier node
alt, err := node.Prompt(ctx, "What about the architecture?")
// node now has two children: node2 and alt

// Streaming variant
stream, err := node.PromptStream(ctx, "Tell me more")
for event := range stream.Events() {
    fmt.Print(event.Content)
}
resultNode, err := stream.Node() // get final Node after stream completes

// Get the full DAG
dag, err := client.GetDAG(ctx, dagID)
for _, n := range dag.Nodes {
    fmt.Printf("[%s] %s: %s\n", n.ID[:8], n.Type, n.Content[:50])
}

// Get a specific node and prompt from it
someNode, err := client.GetNode(ctx, dagID, nodeID)
reply, err := someNode.Prompt(ctx, "Expand on this point")

// List all DAGs
dags, err := client.ListDAGs(ctx)
```

### Key Changes

| Current | Proposed | Why |
|---------|----------|-----|
| `client.Chat(ctx, req, handler)` | `client.Prompt(ctx, msg, opts...)` | Start a new DAG |
| `client.ChatStream(ctx, req, handler)` | `client.PromptStream(ctx, msg, opts...)` | Start a new DAG, streaming |
| `client.ContinueChat(ctx, dagID, req, handler)` | `node.Prompt(ctx, msg)` | Continue from any node |
| `client.ContinueChatStream(ctx, dagID, req, handler)` | `node.PromptStream(ctx, msg)` | Continue from any node, streaming |
| `client.ForkChat(ctx, dagID, req, handler)` | `node.Prompt(ctx, msg)` | Same as continue — fork is just prompting a non-leaf |
| `client.ForkChatStream(ctx, dagID, req, handler)` | `node.PromptStream(ctx, msg)` | Same as continue, streaming |
| SSE callback handler | `stream.Events()` channel | More idiomatic Go |
| `NewChatRequest{Message, SystemPrompt}` | `client.Prompt(ctx, msg, WithSystem(...))` | Functional options |
| `ForkChatRequest{NodeID, Message}` | `node.Prompt(ctx, msg)` | Node knows its ID and DAG |
| _(no equivalent)_ | `client.GetNode(ctx, id)` | Fetch a single node, ready to prompt |

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

    client    *Client   // unexported — enables Prompt()
}

type Stream struct { /* ... */ }
func (s *Stream) Events() <-chan SSEEvent  // channel-based iteration
func (s *Stream) Node() (*Node, error)    // final Node after stream ends

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
// Start new DAG
func (c *Client) Prompt(ctx, message, ...PromptOption) (*Node, error)
func (c *Client) PromptStream(ctx, message, ...PromptOption) (*Stream, error)

// DAG operations
func (c *Client) GetDAG(ctx, id) (*DAG, error)
func (c *Client) ListDAGs(ctx) ([]DAG, error)
func (c *Client) DeleteDAG(ctx, id) error

// Node operations
func (c *Client) GetNode(ctx, dagID, nodeID) (*Node, error)

// Health
func (c *Client) Health(ctx) error
```

### Node Methods

```go
// Continue the DAG from this node
func (n *Node) Prompt(ctx, message) (*Node, error)
func (n *Node) PromptStream(ctx, message) (*Stream, error)

// Navigation (available when DAG was fetched with nodes)
func (n *Node) Children() []Node
```

### Prompt Options (for client.Prompt / client.PromptStream only)

```go
func WithSystem(prompt string) PromptOption
func WithModel(model string) PromptOption
```

---

## Phase 1: Implement Go SDK API

Rewrite the Go SDK with the new node-centric interface.

- [ ] 1a: Define new `Node`, `Stream`, `DAG` types and `Client`/`Node` method signatures
- [ ] 1b: Implement `Client.Prompt`, `Client.PromptStream`, `Node.Prompt`, `Node.PromptStream`
- [ ] 1c: Implement `Client.GetNode` (may require a new server endpoint or use GetDAG + filter)
- [ ] 1d: Update Go SDK unit tests

---

## Phase 2: Update Go Example

- [ ] 2a: Rewrite `examples/go/main.go` to use `Prompt`/`PromptStream` API

---

## Phase 3: Align Python SDK

Port the same pattern to Python: `client.prompt()` / `node.prompt()` / `node.prompt_stream()`.

- [ ] 3a: Redesign Python SDK with `Node.prompt()` / `Node.prompt_stream()` pattern
- [ ] 3b: Update Python SDK unit tests
- [ ] 3c: Rewrite `examples/python/example.py`

---

## Phase 4: Align TypeScript SDK

Port the same pattern to TypeScript: `client.prompt()` / `node.prompt()` / `node.promptStream()`.

- [ ] 4a: Redesign TypeScript SDK with `node.prompt()` / `node.promptStream()` pattern
- [ ] 4b: Update TypeScript SDK unit tests
- [ ] 4c: Rewrite `examples/typescript/example.ts`

---

## Phase 5: Update Website & Docs

- [ ] 5a: Update website code examples to use new API
- [ ] 5b: Update README SDK sections
- [ ] 5c: Update OpenAPI spec if any endpoints changed (e.g., GetNode)

---

## Notes

- **Server API**: Existing endpoints stay the same. `GetNode` needs a new `GET /dags/{dagID}/nodes/{nodeID}` endpoint.
- The `Node` struct holds an unexported `client` reference, so `node.Prompt()` works without passing the client around. Nodes returned from `GetDAG()` also get this reference injected.
- Workflow methods (`ListWorkflows`, `CreateWorkflow`, `RunWorkflow`) stay as-is on the client — separate concern.
- Channel-based `stream.Events()` is for the current unidirectional streaming. Bi-directional streams (interrupts) may warrant a different pattern later.
