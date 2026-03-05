"""
Export format types for langdag migration.
"""
from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Optional


def _now_utc() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


@dataclass
class ToolCall:
    """Represents a tool call made by an AI message."""
    id: str
    name: str
    input: dict

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "name": self.name,
            "input": self.input,
        }


@dataclass
class ExportMessage:
    """A single message in an exported thread."""
    id: str
    role: str  # "user", "assistant", "tool", "system"
    content: str
    created_at: Optional[str] = None
    # assistant-specific
    model: Optional[str] = None
    tokens_in: Optional[int] = None
    tokens_out: Optional[int] = None
    tool_calls: list[ToolCall] = field(default_factory=list)
    # tool-specific
    tool_call_id: Optional[str] = None
    tool_name: Optional[str] = None

    def to_dict(self) -> dict:
        d: dict = {
            "id": self.id,
            "role": self.role,
            "content": self.content,
        }
        if self.created_at is not None:
            d["created_at"] = self.created_at
        if self.model is not None:
            d["model"] = self.model
        if self.tokens_in is not None:
            d["tokens_in"] = self.tokens_in
        if self.tokens_out is not None:
            d["tokens_out"] = self.tokens_out
        if self.tool_calls:
            d["tool_calls"] = [tc.to_dict() for tc in self.tool_calls]
        if self.tool_call_id is not None:
            d["tool_call_id"] = self.tool_call_id
        if self.tool_name is not None:
            d["tool_name"] = self.tool_name
        return d


@dataclass
class ExportThread:
    """A single conversation thread."""
    thread_id: str
    messages: list[ExportMessage] = field(default_factory=list)
    created_at: Optional[str] = None
    metadata: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        d: dict = {
            "thread_id": self.thread_id,
        }
        if self.created_at is not None:
            d["created_at"] = self.created_at
        d["messages"] = [m.to_dict() for m in self.messages]
        d["metadata"] = self.metadata
        return d


@dataclass
class ExportData:
    """Top-level export container."""
    threads: list[ExportThread] = field(default_factory=list)
    version: str = "1"
    source_type: str = "langgraph"
    exported_at: str = field(default_factory=_now_utc)

    def to_dict(self) -> dict:
        return {
            "version": self.version,
            "source_type": self.source_type,
            "exported_at": self.exported_at,
            "threads": [t.to_dict() for t in self.threads],
        }

    def to_json(self, indent: int = 2) -> str:
        return json.dumps(self.to_dict(), indent=indent)

    def save(self, path: str) -> None:
        """Save export to a JSON file."""
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(self.to_json())
