# LangDAG TypeScript SDK

TypeScript client for the [LangDAG](https://github.com/your-org/langdag) REST API.

LangDAG manages LLM conversations as directed acyclic graphs (DAGs), enabling branching, forking, and workflow execution.

## Installation

```bash
npm install langdag
```

## Quick Start

```typescript
import { LangDAGClient } from 'langdag';

const client = new LangDAGClient({
  baseUrl: 'http://localhost:8080',
  apiKey: 'your-api-key',
});

// Start a new chat
const response = await client.chat({
  message: 'Hello, how are you?',
  model: 'claude-sonnet-4-20250514',
});

console.log(response.content);
console.log(`DAG ID: ${response.dag_id}`);
```

## Streaming

All chat methods support streaming via Server-Sent Events (SSE):

```typescript
// Stream a response
for await (const event of client.chat({
  message: 'Tell me a story',
  stream: true
})) {
  switch (event.type) {
    case 'start':
      console.log(`Started DAG: ${event.dag_id}`);
      break;
    case 'delta':
      process.stdout.write(event.content);
      break;
    case 'done':
      console.log(`\nCompleted node: ${event.node_id}`);
      break;
    case 'error':
      console.error(`Error: ${event.error}`);
      break;
  }
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

### DAG Methods

```typescript
// List all DAGs
const dags = await client.listDags();

// Get a specific DAG with all nodes
const dag = await client.getDag('dag-id-or-prefix');

// Delete a DAG
await client.deleteDag('dag-id');
```

### Chat Methods

```typescript
// Start a new conversation
const response = await client.chat({
  message: 'Hello!',
  model?: 'claude-sonnet-4-20250514',
  system_prompt?: 'You are a helpful assistant.',
  stream?: false,
});

// Continue an existing conversation
const response = await client.continueChat('dag-id', {
  message: 'Tell me more',
  stream?: false,
});

// Fork from a specific node (create alternative branch)
const response = await client.forkChat('dag-id', {
  node_id: 'node-id-to-fork-from',
  message: 'What if instead...',
  stream?: false,
});
```

### Workflow Methods

```typescript
// List all workflows
const workflows = await client.listWorkflows();

// Create a workflow
const workflow = await client.createWorkflow({
  name: 'my-workflow',
  description: 'A sample workflow',
  nodes: [
    { id: 'input', type: 'input' },
    { id: 'process', type: 'llm', prompt: 'Process: {{input}}' },
    { id: 'output', type: 'output' },
  ],
  edges: [
    { from: 'input', to: 'process' },
    { from: 'process', to: 'output' },
  ],
});

// Run a workflow
const result = await client.runWorkflow('workflow-id-or-name', {
  input: { key: 'value' },
  stream?: false,
});
```

## Error Handling

The SDK provides typed errors for different failure scenarios:

```typescript
import {
  LangDAGClient,
  NotFoundError,
  UnauthorizedError,
  BadRequestError,
  NetworkError
} from 'langdag';

try {
  await client.getDag('non-existent');
} catch (error) {
  if (error instanceof NotFoundError) {
    console.log('DAG not found');
  } else if (error instanceof UnauthorizedError) {
    console.log('Invalid API key');
  } else if (error instanceof BadRequestError) {
    console.log('Invalid request:', error.message);
  } else if (error instanceof NetworkError) {
    console.log('Network error:', error.message);
  }
}
```

## SSE Utilities

For advanced use cases, you can use the SSE parsing utilities directly:

```typescript
import { parseSSEStream, collectStreamContent } from 'langdag';

// Collect all streamed content into a single result
const stream = client.chat({ message: 'Hello', stream: true });
const { dagId, nodeId, content } = await collectStreamContent(stream);
```

## Types

All request and response types are exported:

```typescript
import type {
  DAG,
  DAGDetail,
  Node,
  ChatResponse,
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
