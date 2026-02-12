"""LangDAG Python SDK.

A Python client library for interacting with the LangDAG API.

Example:
    >>> from langdag import LangDAGClient
    >>>
    >>> # Synchronous client
    >>> with LangDAGClient() as client:
    ...     response = client.prompt("Hello!")
    ...     print(response.content)
    >>>
    >>> # Streaming
    >>> with LangDAGClient() as client:
    ...     for event in client.prompt("Tell me a story", stream=True):
    ...         if event.content:
    ...             print(event.content, end="")

Async Example:
    >>> from langdag import AsyncLangDAGClient
    >>> import asyncio
    >>>
    >>> async def main():
    ...     async with AsyncLangDAGClient() as client:
    ...         response = await client.prompt("Hello!")
    ...         print(response.content)
    >>>
    >>> asyncio.run(main())
"""

from .async_client import AsyncLangDAGClient
from .client import LangDAGClient
from .exceptions import (
    APIError,
    AuthenticationError,
    BadRequestError,
    ConnectionError,
    LangDAGError,
    NotFoundError,
    StreamError,
)
from .models import (
    Node,
    NodeType,
    PromptResponse,
    RunWorkflowResponse,
    SSEEvent,
    SSEEventType,
    ToolDefinition,
    Workflow,
    WorkflowDefaults,
    WorkflowEdge,
    WorkflowNode,
    WorkflowNodeType,
)

__version__ = "0.1.0"

__all__ = [
    # Clients
    "LangDAGClient",
    "AsyncLangDAGClient",
    # Models
    "Node",
    "NodeType",
    "PromptResponse",
    "SSEEvent",
    "SSEEventType",
    "Workflow",
    "WorkflowDefaults",
    "ToolDefinition",
    "WorkflowNode",
    "WorkflowNodeType",
    "WorkflowEdge",
    "RunWorkflowResponse",
    # Exceptions
    "LangDAGError",
    "APIError",
    "AuthenticationError",
    "NotFoundError",
    "BadRequestError",
    "ConnectionError",
    "StreamError",
]
