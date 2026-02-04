"""LangDAG SDK data models."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any


class DAGStatus(str, Enum):
    """Status of a DAG."""

    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class NodeType(str, Enum):
    """Type of a node in a DAG."""

    USER = "user"
    ASSISTANT = "assistant"
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"
    LLM = "llm"
    INPUT = "input"
    OUTPUT = "output"


class WorkflowNodeType(str, Enum):
    """Type of a node in a workflow definition."""

    LLM = "llm"
    TOOL = "tool"
    BRANCH = "branch"
    MERGE = "merge"
    INPUT = "input"
    OUTPUT = "output"


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
    def dag_id(self) -> str | None:
        """Get the DAG ID from start or done events."""
        return self.data.get("dag_id")

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
    """A node in a DAG."""

    id: str
    sequence: int
    node_type: NodeType
    content: str
    created_at: datetime
    parent_id: str | None = None
    model: str | None = None
    tokens_in: int | None = None
    tokens_out: int | None = None
    latency_ms: int | None = None
    status: str | None = None

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
            model=data.get("model"),
            tokens_in=data.get("tokens_in"),
            tokens_out=data.get("tokens_out"),
            latency_ms=data.get("latency_ms"),
            status=data.get("status"),
        )


@dataclass
class DAG:
    """A DAG (conversation or workflow run)."""

    id: str
    status: DAGStatus
    created_at: datetime
    updated_at: datetime
    title: str | None = None
    workflow_id: str | None = None
    model: str | None = None
    system_prompt: str | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> DAG:
        """Create a DAG from a dictionary."""
        return cls(
            id=data["id"],
            status=DAGStatus(data["status"]),
            created_at=_parse_datetime(data["created_at"]),
            updated_at=_parse_datetime(data["updated_at"]),
            title=data.get("title"),
            workflow_id=data.get("workflow_id"),
            model=data.get("model"),
            system_prompt=data.get("system_prompt"),
        )


@dataclass
class DAGDetail(DAG):
    """A DAG with its nodes."""

    node_count: int = 0
    nodes: list[Node] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> DAGDetail:
        """Create a DAGDetail from a dictionary."""
        nodes = [Node.from_dict(n) for n in data.get("nodes", [])]
        return cls(
            id=data["id"],
            status=DAGStatus(data["status"]),
            created_at=_parse_datetime(data["created_at"]),
            updated_at=_parse_datetime(data["updated_at"]),
            title=data.get("title"),
            workflow_id=data.get("workflow_id"),
            model=data.get("model"),
            system_prompt=data.get("system_prompt"),
            node_count=data.get("node_count", len(nodes)),
            nodes=nodes,
        )


@dataclass
class ChatResponse:
    """Response from a chat request."""

    dag_id: str
    node_id: str
    content: str
    tokens_in: int | None = None
    tokens_out: int | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> ChatResponse:
        """Create a ChatResponse from a dictionary."""
        return cls(
            dag_id=data["dag_id"],
            node_id=data["node_id"],
            content=data["content"],
            tokens_in=data.get("tokens_in"),
            tokens_out=data.get("tokens_out"),
        )


@dataclass
class Workflow:
    """A workflow template."""

    id: str
    name: str
    version: int
    created_at: datetime
    updated_at: datetime
    description: str | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Workflow:
        """Create a Workflow from a dictionary."""
        return cls(
            id=data["id"],
            name=data["name"],
            version=data["version"],
            created_at=_parse_datetime(data["created_at"]),
            updated_at=_parse_datetime(data["updated_at"]),
            description=data.get("description"),
        )


@dataclass
class WorkflowDefaults:
    """Default settings for a workflow."""

    provider: str | None = None
    model: str | None = None
    max_tokens: int | None = None
    temperature: float | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to a dictionary, excluding None values."""
        return {k: v for k, v in self.__dict__.items() if v is not None}


@dataclass
class ToolDefinition:
    """A tool definition for a workflow."""

    name: str
    description: str
    input_schema: dict[str, Any]

    def to_dict(self) -> dict[str, Any]:
        """Convert to a dictionary."""
        return {
            "name": self.name,
            "description": self.description,
            "input_schema": self.input_schema,
        }


@dataclass
class WorkflowNode:
    """A node in a workflow definition."""

    id: str
    type: WorkflowNodeType
    content: dict[str, Any] | None = None
    model: str | None = None
    system: str | None = None
    prompt: str | None = None
    tools: list[str] | None = None
    handler: str | None = None
    condition: str | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to a dictionary, excluding None values."""
        result: dict[str, Any] = {"id": self.id, "type": self.type.value}
        for key in [
            "content",
            "model",
            "system",
            "prompt",
            "tools",
            "handler",
            "condition",
        ]:
            value = getattr(self, key)
            if value is not None:
                result[key] = value
        return result


@dataclass
class WorkflowEdge:
    """An edge in a workflow definition."""

    from_node: str
    to_node: str
    condition: str | None = None
    transform: str | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to a dictionary."""
        result: dict[str, Any] = {"from": self.from_node, "to": self.to_node}
        if self.condition is not None:
            result["condition"] = self.condition
        if self.transform is not None:
            result["transform"] = self.transform
        return result


@dataclass
class RunWorkflowResponse:
    """Response from running a workflow."""

    dag_id: str
    status: DAGStatus
    output: dict[str, Any] | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> RunWorkflowResponse:
        """Create a RunWorkflowResponse from a dictionary."""
        return cls(
            dag_id=data["dag_id"],
            status=DAGStatus(data["status"]),
            output=data.get("output"),
        )


def _parse_datetime(value: str | datetime) -> datetime:
    """Parse a datetime string or return as-is if already a datetime."""
    if isinstance(value, datetime):
        return value
    # Handle ISO format with optional timezone
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return datetime.fromisoformat(value)
