"""LangDAG SDK data models."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from typing import Any


class NodeType(str, Enum):
    """Type of a node."""

    USER = "user"
    ASSISTANT = "assistant"
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"


class SSEEventType(str, Enum):
    """Type of Server-Sent Event."""

    START = "start"
    DELTA = "delta"
    DONE = "done"
    ERROR = "error"


@dataclass
class SSEEvent:
    """A Server-Sent Event from a streaming response."""

    event: SSEEventType
    data: dict[str, Any]

    @property
    def node_id(self) -> str | None:
        """Get the node ID from done events."""
        return self.data.get("node_id")

    @property
    def content(self) -> str | None:
        """Get the content from delta events."""
        return self.data.get("content")


@dataclass
class Node:
    """A node in a conversation tree."""

    id: str
    sequence: int
    node_type: NodeType
    content: str
    created_at: datetime
    parent_id: str | None = None
    root_id: str | None = None
    model: str | None = None
    tokens_in: int | None = None
    tokens_out: int | None = None
    cache_read_tokens_in: int | None = None
    cache_creation_tokens_in: int | None = None
    reasoning_tokens: int | None = None
    latency_ms: int | None = None
    status: str | None = None
    title: str | None = None
    system_prompt: str | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Node:
        """Create a Node from a dictionary."""
        return cls(
            id=data["id"],
            sequence=data["sequence"],
            node_type=NodeType(data["node_type"]),
            content=data["content"],
            created_at=_parse_datetime(data["created_at"]),
            parent_id=data.get("parent_id"),
            root_id=data.get("root_id"),
            model=data.get("model"),
            tokens_in=data.get("tokens_in"),
            tokens_out=data.get("tokens_out"),
            cache_read_tokens_in=data.get("tokens_cache_read"),
            cache_creation_tokens_in=data.get("tokens_cache_creation"),
            reasoning_tokens=data.get("tokens_reasoning"),
            latency_ms=data.get("latency_ms"),
            status=data.get("status"),
            title=data.get("title"),
            system_prompt=data.get("system_prompt"),
        )


@dataclass
class PromptResponse:
    """Response from a prompt request."""

    node_id: str
    content: str
    tokens_in: int | None = None
    tokens_out: int | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> PromptResponse:
        """Create a PromptResponse from a dictionary."""
        return cls(
            node_id=data["node_id"],
            content=data["content"],
            tokens_in=data.get("tokens_in"),
            tokens_out=data.get("tokens_out"),
        )


def _parse_datetime(value: str | datetime) -> datetime:
    """Parse a datetime string or return as-is if already a datetime."""
    if isinstance(value, datetime):
        return value
    # Handle ISO format with optional timezone
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return datetime.fromisoformat(value)
