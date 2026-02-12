# LangDAG TypeScript SDK Example

This example demonstrates how to use the LangDAG TypeScript SDK to manage LLM conversations as node trees.

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

1. **Start a Conversation** - Creates a new tree with `client.prompt()`
2. **Stream a Response** - Uses `client.promptStream()` with async iteration
3. **Continue from a Node** - Extends a conversation with `node.prompt()`
4. **Branch from a Node** - Creates an alternative path by prompting from an earlier node
5. **List Roots** - Shows all conversations with `client.listRoots()`
6. **View Tree** - Displays the branching tree structure with `client.getTree()`

## Key SDK Features Used

- `LangDAGClient` - Main client for API interactions
- `client.prompt()` / `client.promptStream()` - Start new conversations
- `node.prompt()` / `node.promptStream()` - Continue from any node
- `client.listRoots()` - List root nodes (conversations)
- `client.getTree()` - Get full tree from a node
