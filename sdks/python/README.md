# LangDAG Python SDK

Python client library for the [LangDAG](https://langdag.com) API - manage LLM conversations as node trees.

## Installation

```bash
pip install langdag
```

## Quick Start

### Synchronous Client

```python
from langdag import LangDAGClient

client = LangDAGClient(base_url="http://localhost:8080")

# Start a conversation
node = client.prompt("Hello, how are you?")
print(node.content)

# Continue from any node
node2 = node.prompt("Tell me more")
print(node2.content)

# Close the client when done
client.close()
```

### Using Context Manager

```python
from langdag import LangDAGClient

with LangDAGClient() as client:
    node = client.prompt("Hello!")
    print(node.content)
```

### Async Client

```python
import asyncio
from langdag import AsyncLangDAGClient

async def main():
    async with AsyncLangDAGClient() as client:
        node = await client.prompt("Hello!")
        print(node.content)

asyncio.run(main())
```

### Streaming Responses

```python
from langdag import LangDAGClient, SSEEventType

with LangDAGClient() as client:
    # Stream a new conversation
    for event in client.prompt("Tell me a story", stream=True):
        if event.content:
            print(event.content, end="", flush=True)

    # Stream from an existing node
    for event in node.prompt_stream("Explain in detail"):
        if event.content:
            print(event.content, end="", flush=True)
```

## API Reference

### LangDAGClient / AsyncLangDAGClient

#### Constructor

```python
LangDAGClient(
    base_url: str = "http://localhost:8080",
    api_key: str | None = None,
    timeout: float = 60.0
)
```

#### Prompt Methods

- `prompt(message, model=None, system_prompt=None, stream=False)` - Start a new conversation (returns `Node` or event iterator)
- `node.prompt(message)` - Continue from any node (returns `Node`)
- `node.prompt_stream(message)` - Stream from any node (returns event iterator)

#### Node Methods

- `list_roots()` - List root nodes (conversations)
- `get_node(node_id)` - Get a node by ID
- `get_tree(node_id)` - Get full tree from a node
- `delete_node(node_id)` - Delete a node and its subtree

#### Workflow Methods

- `list_workflows()` - List all workflow templates
- `create_workflow(name, nodes, ...)` - Create a workflow
- `run_workflow(workflow_id, input=None, stream=False)` - Run a workflow

### Streaming Events

When `stream=True`, methods return an iterator of `SSEEvent` objects:

- `SSEEventType.NODE_INFO` - Node metadata
- `SSEEventType.DELTA` - Content chunk, access via `event.content`
- `SSEEventType.DONE` - Stream complete
- `SSEEventType.ERROR` - Error occurred

### Models

- `Node` - A node in a conversation tree (has `.prompt()` and `.prompt_stream()` methods)
- `Tree` - A tree of nodes
- `Workflow` - A workflow template
- `RunWorkflowResponse` - Response from running a workflow

### Exceptions

- `LangDAGError` - Base exception
- `APIError` - API returned an error
- `AuthenticationError` - Authentication failed (401)
- `NotFoundError` - Resource not found (404)
- `BadRequestError` - Invalid request (400)
- `ConnectionError` - Failed to connect to server

## License

MIT
