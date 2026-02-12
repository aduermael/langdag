# LangDAG TypeScript SDK

TypeScript client for the [LangDAG](https://github.com/aduermael/langdag) REST API.

LangDAG manages LLM conversations as node trees, enabling branching and workflow execution.

## Installation

```bash
npm install langdag
```

## Quick Start

```typescript
import { LangDAGClient } from 'langdag';

const client = new LangDAGClient({
  baseUrl: 'http://localhost:8080',
});

// Start a new conversation
const node = await client.prompt('Hello, how are you?');
console.log(node.content);

// Continue from any node
const node2 = await node.prompt('Tell me more');
console.log(node2.content);
```

## Streaming

```typescript
// Stream a new conversation
const stream = await client.promptStream('Tell me a story');
for await (const event of stream.events()) {
  process.stdout.write(event.content);
}
const result = await stream.node();
console.log(`\nNode ID: ${result.id}`);

// Stream from an existing node
const stream2 = await node.promptStream('Explain in detail');
for await (const event of stream2.events()) {
  process.stdout.write(event.content);
}
```

## API Reference

### Constructor

```typescript
const client = new LangDAGClient({
  baseUrl?: string;    // Default: 'http://localhost:8080'
  apiKey?: string;     // X-API-Key header
  bearerToken?: string; // Authorization: Bearer header
  fetch?: typeof fetch; // Custom fetch implementation
});
```

### Prompt Methods

```typescript
// Start a new conversation (returns Node)
const node = await client.prompt('Hello!');
const node = await client.prompt('Hello!', {
  model: 'claude-sonnet-4-20250514',
  systemPrompt: 'You are helpful.',
});

// Continue from any node
const node2 = await node.prompt('Tell me more');

// Branch from an earlier node
const alt = await node.prompt('Different angle');

// Stream a new conversation
const stream = await client.promptStream('Tell me a story');
for await (const event of stream.events()) { ... }
const result = await stream.node();

// Stream from a node
const stream = await node.promptStream('Explain');
```

### Node Operations

```typescript
// Get a node by ID
const node = await client.getNode('node-id');

// Get full tree from a node
const tree = await client.getTree('node-id');
for (const n of tree.nodes) {
  console.log(`[${n.type}] ${n.content}`);
}

// List root nodes (conversations)
const roots = await client.listRoots();

// Delete a node and its subtree
await client.deleteNode('node-id');
```

### Workflow Methods

```typescript
// List all workflows
const workflows = await client.listWorkflows();

// Create a workflow
const workflow = await client.createWorkflow({...});

// Run a workflow
const result = await client.runWorkflow('workflow-id', {...});
```

## Error Handling

The SDK provides typed errors for different failure scenarios:

```typescript
import {
  LangDAGClient,
  NotFoundError,
  UnauthorizedError,
  BadRequestError,
  NetworkError,
} from 'langdag';

try {
  await client.getNode('non-existent');
} catch (error) {
  if (error instanceof NotFoundError) {
    console.log('Node not found');
  } else if (error instanceof UnauthorizedError) {
    console.log('Invalid API key');
  }
}
```

## Types

All request and response types are exported:

```typescript
import type {
  Node,
  NodeData,
  SSEEvent,
  Workflow,
  // ... and more
} from 'langdag';
```

## Requirements

- Node.js 18+ (uses native `fetch`)
- TypeScript 5.0+ (for development)

## License

MIT
