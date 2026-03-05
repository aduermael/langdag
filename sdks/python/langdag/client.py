"""Synchronous LangDAG client."""

from __future__ import annotations

import json
from typing import Any, Iterator

import httpx

from .exceptions import (
    APIError,
    AuthenticationError,
    BadRequestError,
    ConnectionError,
    NotFoundError,
    StreamError,
)
from .models import (
    Node,
    PromptResponse,
    SSEEvent,
    SSEEventType,
)


class LangDAGClient:
    """Synchronous client for the LangDAG API.

    Example:
        >>> client = LangDAGClient()
        >>> response = client.prompt("Hello, world!")
        >>> print(response.content)

        >>> # Streaming
        >>> for event in client.prompt("Tell me a story", stream=True):
        ...     if event.content:
        ...         print(event.content, end="")
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8080",
        api_key: str | None = None,
        timeout: float = 60.0,
    ) -> None:
        """Initialize the client.

        Args:
            base_url: Base URL of the LangDAG API server.
            api_key: Optional API key for authentication.
            timeout: Request timeout in seconds.
        """
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout
        self._client: httpx.Client | None = None

    def _get_client(self) -> httpx.Client:
        """Get or create the HTTP client."""
        if self._client is None:
            headers: dict[str, str] = {}
            if self.api_key:
                headers["X-API-Key"] = self.api_key
            self._client = httpx.Client(
                base_url=self.base_url,
                headers=headers,
                timeout=self.timeout,
            )
        return self._client

    def close(self) -> None:
        """Close the HTTP client."""
        if self._client is not None:
            self._client.close()
            self._client = None

    def __enter__(self) -> LangDAGClient:
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()

    def _handle_response(self, response: httpx.Response) -> dict[str, Any]:
        """Handle an HTTP response, raising appropriate exceptions for errors."""
        if response.status_code == 401:
            try:
                body = response.json()
                message = body.get("error", "Authentication failed")
            except Exception:
                message = "Authentication failed"
            raise AuthenticationError(message, response.status_code)

        if response.status_code == 404:
            try:
                body = response.json()
                message = body.get("error", "Resource not found")
            except Exception:
                message = "Resource not found"
            raise NotFoundError(message, response.status_code)

        if response.status_code == 400:
            try:
                body = response.json()
                message = body.get("error", "Bad request")
            except Exception:
                message = "Bad request"
            raise BadRequestError(message, response.status_code)

        if response.status_code >= 400:
            try:
                body = response.json()
                message = body.get("error", f"API error: {response.status_code}")
            except Exception:
                message = f"API error: {response.status_code}"
            raise APIError(message, response.status_code)

        return response.json()

    def _request(
        self,
        method: str,
        path: str,
        json_body: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Make an HTTP request."""
        try:
            client = self._get_client()
            response = client.request(method, path, json=json_body)
            return self._handle_response(response)
        except httpx.ConnectError as e:
            raise ConnectionError(f"Failed to connect to {self.base_url}: {e}") from e

    def _stream_request(
        self,
        method: str,
        path: str,
        json_body: dict[str, Any] | None = None,
    ) -> Iterator[SSEEvent]:
        """Make a streaming HTTP request and yield SSE events."""
        try:
            client = self._get_client()
            with client.stream(
                method,
                path,
                json=json_body,
                headers={"Accept": "text/event-stream"},
            ) as response:
                if response.status_code >= 400:
                    # For error responses, read the body and handle
                    response.read()
                    self._handle_response(response)

                yield from _parse_sse_stream(response.iter_lines())
        except httpx.ConnectError as e:
            raise ConnectionError(f"Failed to connect to {self.base_url}: {e}") from e

    # --- Health ---

    def health(self) -> dict[str, str]:
        """Check the server health.

        Returns:
            Health status dictionary with 'status' key.
        """
        return self._request("GET", "/health")

    # --- Node Methods ---

    def list_roots(self) -> list[Node]:
        """List all root nodes (conversation roots).

        Returns:
            List of root Node objects.
        """
        data = self._request("GET", "/nodes")
        return [Node.from_dict(n) for n in data]

    def get_node(self, node_id: str) -> Node:
        """Get a single node by ID.

        Args:
            node_id: Node ID (full or prefix).

        Returns:
            Node object.

        Raises:
            NotFoundError: If the node is not found.
        """
        data = self._request("GET", f"/nodes/{node_id}")
        return Node.from_dict(data)

    def get_tree(self, node_id: str) -> list[Node]:
        """Get the full tree of nodes rooted at the given node.

        Args:
            node_id: Node ID (full or prefix).

        Returns:
            List of Node objects in the tree.

        Raises:
            NotFoundError: If the node is not found.
        """
        data = self._request("GET", f"/nodes/{node_id}/tree")
        return [Node.from_dict(n) for n in data]

    def delete_node(self, node_id: str) -> dict[str, str]:
        """Delete a node and its descendants.

        Args:
            node_id: Node ID (full or prefix).

        Returns:
            Dictionary with 'status' and 'id' keys.

        Raises:
            NotFoundError: If the node is not found.
        """
        return self._request("DELETE", f"/nodes/{node_id}")

    # --- Prompt Methods ---

    def prompt(
        self,
        message: str,
        model: str | None = None,
        system_prompt: str | None = None,
        stream: bool = False,
        tools: list[dict[str, Any]] | None = None,
    ) -> PromptResponse | Iterator[SSEEvent]:
        """Send a prompt to start a new conversation.

        Args:
            message: The message to send.
            model: LLM model to use.
            system_prompt: Optional system prompt.
            stream: If True, return an iterator of SSE events.
            tools: Optional list of tool definitions for the LLM.

        Returns:
            PromptResponse if stream=False, otherwise an iterator of SSEEvent.

        Example:
            >>> # Non-streaming
            >>> response = client.prompt("Hello!")
            >>> print(response.content)

            >>> # Streaming
            >>> for event in client.prompt("Hello!", stream=True):
            ...     if event.content:
            ...         print(event.content, end="")
        """
        body: dict[str, Any] = {"message": message, "stream": stream}
        if model is not None:
            body["model"] = model
        if system_prompt is not None:
            body["system_prompt"] = system_prompt
        if tools is not None:
            body["tools"] = tools

        if stream:
            return self._stream_request("POST", "/prompt", body)
        else:
            data = self._request("POST", "/prompt", body)
            return PromptResponse.from_dict(data)

    def prompt_from(
        self,
        node_id: str,
        message: str,
        model: str | None = None,
        stream: bool = False,
        tools: list[dict[str, Any]] | None = None,
    ) -> PromptResponse | Iterator[SSEEvent]:
        """Send a prompt continuing from an existing node.

        Args:
            node_id: Node ID to continue from.
            message: The message to send.
            model: LLM model to use.
            stream: If True, return an iterator of SSE events.
            tools: Optional list of tool definitions for the LLM.

        Returns:
            PromptResponse if stream=False, otherwise an iterator of SSEEvent.

        Raises:
            NotFoundError: If the node is not found.
        """
        body: dict[str, Any] = {"message": message, "stream": stream}
        if model is not None:
            body["model"] = model
        if tools is not None:
            body["tools"] = tools

        if stream:
            return self._stream_request("POST", f"/nodes/{node_id}/prompt", body)
        else:
            data = self._request("POST", f"/nodes/{node_id}/prompt", body)
            return PromptResponse.from_dict(data)

    # --- Alias Methods ---

    def create_alias(self, node_id: str, alias: str) -> dict[str, str]:
        """Create a human-readable alias for a node.

        Args:
            node_id: Node ID (full or prefix).
            alias: The alias string.

        Returns:
            Dictionary with 'alias' and 'node_id' keys.
        """
        return self._request("PUT", f"/nodes/{node_id}/aliases/{alias}")

    def delete_alias(self, alias: str) -> dict[str, str]:
        """Delete an alias.

        Args:
            alias: The alias to delete.

        Returns:
            Dictionary with 'status' key.
        """
        return self._request("DELETE", f"/aliases/{alias}")

    def list_aliases(self, node_id: str) -> list[str]:
        """List all aliases for a node.

        Args:
            node_id: Node ID (full or prefix).

        Returns:
            List of alias strings.
        """
        data = self._request("GET", f"/nodes/{node_id}/aliases")
        return data.get("aliases", [])


def _parse_sse_stream(lines: Iterator[str]) -> Iterator[SSEEvent]:
    """Parse SSE stream lines into events."""
    event_type: str | None = None
    data_lines: list[str] = []

    for line in lines:
        line = line.rstrip("\r\n")

        if line.startswith("event:"):
            event_type = line[6:].strip()
        elif line.startswith("data:"):
            data_lines.append(line[5:].strip())
        elif line == "":
            # Empty line signals end of event
            if event_type is not None and data_lines:
                data_str = "\n".join(data_lines)
                try:
                    data = json.loads(data_str)
                except json.JSONDecodeError:
                    # For error events, data might be plain text
                    data = {"message": data_str}

                try:
                    sse_event_type = SSEEventType(event_type)
                except ValueError:
                    # Unknown event type, skip
                    event_type = None
                    data_lines = []
                    continue

                yield SSEEvent(event=sse_event_type, data=data)

            event_type = None
            data_lines = []
