"""
Main export logic: converts LangGraph checkpoint data to the langdag export format.
"""
from __future__ import annotations

import json
import uuid
from datetime import datetime, timezone
from typing import Any, Optional

from langgraph_export.types import (
    ExportData,
    ExportMessage,
    ExportThread,
    ToolCall,
)


# ---------------------------------------------------------------------------
# Role mapping
# ---------------------------------------------------------------------------

ROLE_MAP: dict[str, str] = {
    "human": "user",
    "ai": "assistant",
    "tool": "tool",
    "system": "system",
    "function": "tool",  # older LangChain convention
}


# ---------------------------------------------------------------------------
# Message extraction
# ---------------------------------------------------------------------------

def _content_to_str(content: Any) -> str:
    """Normalise message content to a plain string."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        # LangChain multipart content: extract text parts
        parts: list[str] = []
        for part in content:
            if isinstance(part, str):
                parts.append(part)
            elif isinstance(part, dict):
                if part.get("type") == "text":
                    parts.append(part.get("text", ""))
                else:
                    # Keep other parts as JSON so nothing is silently dropped
                    parts.append(json.dumps(part))
        return "\n".join(parts)
    # Fallback
    return json.dumps(content)


def _extract_message(msg: Any) -> Optional[ExportMessage]:
    """
    Convert a LangChain BaseMessage (or a plain dict) to an ExportMessage.

    Returns None for message types we want to skip entirely.
    """
    # Support plain dicts (e.g. when channel_values["messages"] contains raw dicts)
    if isinstance(msg, dict):
        msg_type = msg.get("type", "")
        msg_id = msg.get("id") or str(uuid.uuid4())
        msg_content = _content_to_str(msg.get("content", ""))
        role = ROLE_MAP.get(msg_type, msg_type)
        result = ExportMessage(id=msg_id, role=role, content=msg_content)

        if msg_type == "ai":
            meta = msg.get("response_metadata") or {}
            result.model = (
                meta.get("model_name") or meta.get("model") or meta.get("model_id")
            )
            usage = msg.get("usage_metadata") or {}
            if usage:
                result.tokens_in = usage.get("input_tokens")
                result.tokens_out = usage.get("output_tokens")
            raw_calls = msg.get("tool_calls") or []
            result.tool_calls = [
                ToolCall(
                    id=tc.get("id", str(uuid.uuid4())),
                    name=tc.get("name", ""),
                    input=tc.get("args", tc.get("input", {})),
                )
                for tc in raw_calls
            ]

        if msg_type == "tool":
            result.tool_call_id = msg.get("tool_call_id")
            result.tool_name = msg.get("name")

        return result

    # LangChain message object
    msg_type: str = getattr(msg, "type", "")
    if not msg_type:
        return None

    role = ROLE_MAP.get(msg_type, msg_type)
    msg_id: str = getattr(msg, "id", None) or str(uuid.uuid4())
    content_raw = getattr(msg, "content", "")
    content = _content_to_str(content_raw)

    result = ExportMessage(id=msg_id, role=role, content=content)

    if msg_type == "ai":
        meta: dict = getattr(msg, "response_metadata", None) or {}
        result.model = (
            meta.get("model_name") or meta.get("model") or meta.get("model_id")
        )
        usage: dict = getattr(msg, "usage_metadata", None) or {}
        if usage:
            result.tokens_in = usage.get("input_tokens")
            result.tokens_out = usage.get("output_tokens")
        raw_calls = getattr(msg, "tool_calls", None) or []
        result.tool_calls = [
            ToolCall(
                id=tc.get("id", str(uuid.uuid4())),
                name=tc.get("name", ""),
                input=tc.get("args", tc.get("input", {})),
            )
            for tc in raw_calls
        ]

    if msg_type == "tool":
        result.tool_call_id = getattr(msg, "tool_call_id", None)
        result.tool_name = getattr(msg, "name", None)

    return result


# ---------------------------------------------------------------------------
# Timestamp helpers
# ---------------------------------------------------------------------------

def _ts_from_checkpoint(checkpoint_tuple: Any) -> Optional[str]:
    """
    Try to extract a creation timestamp from a CheckpointTuple.

    LangGraph CheckpointTuple has a .metadata attribute (dict) that may contain
    a "created_at" or "ts" key.  The checkpoint dict itself may also have "ts".
    """
    # checkpoint_tuple.metadata is a dict
    meta: dict = getattr(checkpoint_tuple, "metadata", None) or {}
    for key in ("created_at", "ts"):
        val = meta.get(key)
        if val:
            return _normalise_ts(val)

    # checkpoint_tuple.checkpoint is the inner checkpoint dict
    cp: dict = getattr(checkpoint_tuple, "checkpoint", None) or {}
    for key in ("ts", "created_at"):
        val = cp.get(key)
        if val:
            return _normalise_ts(val)

    # config may carry thread_ts
    config: dict = getattr(checkpoint_tuple, "config", None) or {}
    configurable: dict = config.get("configurable", {})
    val = configurable.get("thread_ts") or configurable.get("checkpoint_ns")
    if val and isinstance(val, str) and "T" in val:
        return _normalise_ts(val)

    return None


def _normalise_ts(ts: Any) -> str:
    """Ensure a timestamp string ends with Z (UTC marker)."""
    if isinstance(ts, datetime):
        if ts.tzinfo is None:
            ts = ts.replace(tzinfo=timezone.utc)
        return ts.strftime("%Y-%m-%dT%H:%M:%SZ")
    s = str(ts)
    if s.endswith("+00:00"):
        s = s[:-6] + "Z"
    if not s.endswith("Z"):
        s = s + "Z"
    return s


def _now_utc() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


# ---------------------------------------------------------------------------
# Core conversion
# ---------------------------------------------------------------------------

def _checkpoint_to_thread(thread_id: str, checkpoint_tuple: Any) -> Optional[ExportThread]:
    """
    Convert a single CheckpointTuple to an ExportThread.

    Returns None if the checkpoint carries no messages.
    """
    cp: dict = getattr(checkpoint_tuple, "checkpoint", None) or {}
    channel_values: dict = cp.get("channel_values", {})

    raw_messages: list = channel_values.get("messages", [])
    if not raw_messages:
        return None

    messages: list[ExportMessage] = []
    for raw_msg in raw_messages:
        em = _extract_message(raw_msg)
        if em is not None:
            messages.append(em)

    if not messages:
        return None

    created_at = _ts_from_checkpoint(checkpoint_tuple) or _now_utc()

    return ExportThread(
        thread_id=thread_id,
        messages=messages,
        created_at=created_at,
        metadata={"original_thread_id": thread_id},
    )


# ---------------------------------------------------------------------------
# LangGraphExporter
# ---------------------------------------------------------------------------

class LangGraphExporter:
    """
    Exports LangGraph conversation data to the langdag JSON format.

    Usage::

        # From InMemorySaver
        exporter = LangGraphExporter.from_memory(saver)
        export = exporter.export()
        export.save("export.json")

        # From SqliteSaver
        exporter = LangGraphExporter.from_sqlite("path/to/langgraph.db")
        export = exporter.export()
        export.save("export.json")

        # From PostgresSaver
        exporter = LangGraphExporter.from_postgres("postgresql://...")
        export = exporter.export()
        export.save("export.json")
    """

    def __init__(self, backend: Any) -> None:
        """
        Parameters
        ----------
        backend:
            Any object that implements ``iter_latest_checkpoints()`` yielding
            ``(thread_id: str, checkpoint_tuple)`` pairs.
        """
        self._backend = backend

    # ------------------------------------------------------------------
    # Factory helpers
    # ------------------------------------------------------------------

    @classmethod
    def from_memory(cls, saver) -> "LangGraphExporter":
        """Create an exporter from a LangGraph InMemorySaver."""
        from langgraph_export.backends.memory import MemoryBackend

        return cls(MemoryBackend(saver))

    @classmethod
    def from_sqlite(cls, db_path: str) -> "LangGraphExporter":
        """Create an exporter from a LangGraph SQLite database path."""
        from langgraph_export.backends.sqlite import SqliteBackend

        return cls(SqliteBackend(db_path))

    @classmethod
    def from_postgres(cls, connection_string: str) -> "LangGraphExporter":
        """Create an exporter from a LangGraph PostgreSQL connection string."""
        from langgraph_export.backends.postgres import PostgresBackend

        return cls(PostgresBackend(connection_string))

    # ------------------------------------------------------------------
    # Export
    # ------------------------------------------------------------------

    def export(self) -> ExportData:
        """
        Read all threads from the backend and return an ExportData object.

        Only threads that contain at least one message are included.
        """
        threads: list[ExportThread] = []

        for thread_id, checkpoint_tuple in self._backend.iter_latest_checkpoints():
            thread = _checkpoint_to_thread(thread_id, checkpoint_tuple)
            if thread is not None:
                threads.append(thread)

        # Sort threads by thread_id for deterministic output
        threads.sort(key=lambda t: t.thread_id)

        return ExportData(threads=threads)
