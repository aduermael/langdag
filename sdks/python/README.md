# LangDAG Python SDK

Python client library for the [LangDAG](https://langdag.com) API - manage LLM conversations as directed acyclic graphs (DAGs).

## Installation

```bash
pip install langdag
```

## Quick Start

### Synchronous Client

```python
from langdag import LangDAGClient

# Create a client
client = LangDAGClient(base_url="http://localhost:8080", api_key="your-api-key")

# Start a conversation
response = client.chat("Hello, how are you?")
print(response.content)
print(f"DAG ID: {response.dag_id}")

# Continue the conversation
response = client.continue_chat(response.dag_id, "Tell me more")
print(response.content)

# Close the client when done
client.close()
```

### Using Context Manager

```python
from langdag import LangDAGClient

with LangDAGClient() as client:
    response = client.chat("Hello!")
    print(response.content)
```

### Async Client

```python
import asyncio
from langdag import AsyncLangDAGClient

async def main():
    async with AsyncLangDAGClient() as client:
        response = await client.chat("Hello!")
        print(response.content)

asyncio.run(main())
```

### Streaming Responses

```python
from langdag import LangDAGClient, SSEEventType

with LangDAGClient() as client:
    # Stream chat response
    for event in client.chat("Tell me a story", stream=True):
        if event.event == SSEEventType.DELTA:
            print(event.content, end="", flush=True)
        elif event.event == SSEEventType.DONE:
            print(f"\n\nDAG ID: {event.dag_id}")
```

### Async Streaming

```python
import asyncio
from langdag import AsyncLangDAGClient, SSEEventType

async def main():
    async with AsyncLangDAGClient() as client:
        async for event in await client.chat("Tell me a story", stream=True):
            if event.event == SSEEventType.DELTA:
                print(event.content, end="", flush=True)

asyncio.run(main())
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

#### DAG Methods

- `list_dags()` - List all DAGs
- `get_dag(dag_id)` - Get a DAG with all its nodes
- `delete_dag(dag_id)` - Delete a DAG

#### Chat Methods

- `chat(message, model=None, system_prompt=None, stream=False)` - Start a new conversation
- `continue_chat(dag_id, message, stream=False)` - Continue an existing conversation
- `fork_chat(dag_id, node_id, message, stream=False)` - Fork a conversation from a specific node

#### Workflow Methods

- `list_workflows()` - List all workflow templates
- `create_workflow(name, nodes, description=None, defaults=None, tools=None, edges=None)` - Create a workflow
- `run_workflow(workflow_id, input=None, stream=False)` - Run a workflow

### Streaming Events

When `stream=True`, methods return an iterator of `SSEEvent` objects:

- `SSEEventType.START` - Stream started, contains `dag_id`
- `SSEEventType.DELTA` - Content chunk, access via `event.content`
- `SSEEventType.DONE` - Stream complete, contains `dag_id` and `node_id`
- `SSEEventType.ERROR` - Error occurred

### Models

- `DAG` - A conversation or workflow run
- `DAGDetail` - DAG with its nodes
- `Node` - A node in a DAG (message, tool call, etc.)
- `ChatResponse` - Response from chat endpoints
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
