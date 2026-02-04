# LangDAG TypeScript SDK Example

This example demonstrates how to use the LangDAG TypeScript SDK to manage LLM conversations as directed acyclic graphs (DAGs).

## Prerequisites

- Node.js 18+
- LangDAG server running locally (default: `http://localhost:8080`)

## Setup

1. Build the SDK (from the repository root):

```bash
cd sdks/typescript
npm install
npm run build
```

2. Install example dependencies:

```bash
cd examples/typescript
npm install
```

3. Start the LangDAG server (in another terminal):

```bash
langdag serve
```

## Running the Example

```bash
npm run dev
```

Or build and run separately:

```bash
npm run build
npm start
```

## Configuration

Set environment variables to customize the connection:

```bash
# API server URL (default: http://localhost:8080)
export LANGDAG_URL=http://localhost:8080

# API key for authentication (optional)
export LANGDAG_API_KEY=your-api-key
```

## What the Example Demonstrates

1. **Start a Chat (Streaming)** - Creates a new conversation with streaming response
2. **Continue Conversation** - Adds follow-up messages to the same DAG
3. **Fork from Node** - Branches from an earlier point to explore an alternative
4. **List DAGs** - Shows all conversations in the system
5. **Visualize Structure** - Displays the branching tree structure of a DAG

## Example Output

```
LangDAG TypeScript SDK Example
==============================

Connecting to: http://localhost:8080
API Key: (not set)

============================================================
  Step 1: Start a New Chat (Streaming)
============================================================

Topic: The history of programming languages

User: Tell me briefly about the history of programming languages...
------------------------------------------------------------
[Assistant] Streaming response:

  (DAG started: a1b2c3d4...)

  Programming languages have evolved dramatically since the 1950s...

  (Completed - Node ID: e5f6g7h8...)
------------------------------------------------------------

...
```

## Key SDK Features Used

- `LangDAGClient` - Main client for API interactions
- `client.chat()` - Start new conversations
- `client.continueChat()` - Continue existing conversations
- `client.forkChat()` - Branch from any node
- `client.listDags()` - List all conversations
- `client.getDag()` - Get full DAG details with nodes
- Streaming via `{ stream: true }` option
- SSE event handling (`start`, `delta`, `done`, `error`)
