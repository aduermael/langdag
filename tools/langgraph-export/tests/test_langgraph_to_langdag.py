"""
Full integration test: LangGraph -> export JSON -> validate langdag import format.

This test verifies the complete migration pipeline:
  1. Create LangGraph conversation data (InMemorySaver)
  2. Export to JSON file via LangGraphExporter
  3. Validate the JSON matches the langdag import schema exactly
  4. (Optionally) invoke the langdag CLI to import if it is installed
"""
from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

import pytest
from langchain_core.messages import HumanMessage
from langgraph.checkpoint.memory import InMemorySaver

from langgraph_export import LangGraphExporter

from conftest import (
    ConversationState,
    create_simple_graph,
    create_tool_use_graph,
)


# ---------------------------------------------------------------------------
# Schema validation helpers
# ---------------------------------------------------------------------------

def _assert_valid_export(data: dict) -> None:
    """Assert that a parsed export dict matches the langdag import schema."""
    assert data["version"] == "1", "version must be '1'"
    assert data["source_type"] == "langgraph", "source_type must be 'langgraph'"
    assert data["exported_at"].endswith("Z"), "exported_at must be a UTC timestamp"
    assert isinstance(data["threads"], list), "threads must be a list"

    for thread in data["threads"]:
        _assert_valid_thread(thread)


def _assert_valid_thread(thread: dict) -> None:
    assert "thread_id" in thread, "thread must have thread_id"
    assert isinstance(thread["messages"], list), "messages must be a list"
    assert "metadata" in thread, "thread must have metadata"
    assert thread["metadata"].get("original_thread_id") == thread["thread_id"]

    for msg in thread["messages"]:
        _assert_valid_message(msg)


def _assert_valid_message(msg: dict) -> None:
    assert "id" in msg and msg["id"], "message must have non-empty id"
    assert msg["role"] in ("user", "assistant", "tool", "system"), (
        f"unexpected role: {msg['role']}"
    )
    assert "content" in msg, "message must have content"

    if msg["role"] == "assistant":
        # tool_calls may be absent if there were none
        if "tool_calls" in msg:
            for tc in msg["tool_calls"]:
                assert "id" in tc
                assert "name" in tc
                assert "input" in tc

    if msg["role"] == "tool":
        assert "tool_call_id" in msg, "tool message must have tool_call_id"
        assert "tool_name" in msg, "tool message must have tool_name"


# ---------------------------------------------------------------------------
# Integration tests
# ---------------------------------------------------------------------------

class TestFullMigrationPipeline:
    def test_single_turn_export_format(self, tmp_path):
        """Export a single-turn conversation and validate the JSON schema."""
        saver = InMemorySaver()
        graph = create_simple_graph(saver)

        thread_config = {"configurable": {"thread_id": "test-thread-1"}}
        graph.invoke(
            {"messages": [HumanMessage(content="What is Python?")]}, thread_config
        )

        exporter = LangGraphExporter.from_memory(saver)
        export = exporter.export()

        export_file = tmp_path / "export.json"
        export.save(str(export_file))

        with open(export_file) as fh:
            data = json.load(fh)

        _assert_valid_export(data)

        assert len(data["threads"]) == 1
        thread = data["threads"][0]
        assert thread["thread_id"] == "test-thread-1"
        assert len(thread["messages"]) == 2
        assert thread["messages"][0]["role"] == "user"
        assert thread["messages"][0]["content"] == "What is Python?"
        assert thread["messages"][1]["role"] == "assistant"

    def test_multi_turn_export_format(self, tmp_path):
        """Export a multi-turn conversation and validate message count."""
        saver = InMemorySaver()
        graph = create_simple_graph(saver)

        cfg = {"configurable": {"thread_id": "multi-turn-thread"}}
        graph.invoke({"messages": [HumanMessage(content="Hello")]}, cfg)
        graph.invoke({"messages": [HumanMessage(content="Tell me more")]}, cfg)
        graph.invoke({"messages": [HumanMessage(content="Thanks")]}, cfg)

        exporter = LangGraphExporter.from_memory(saver)
        export = exporter.export()

        export_file = tmp_path / "multi.json"
        export.save(str(export_file))

        with open(export_file) as fh:
            data = json.load(fh)

        _assert_valid_export(data)
        thread = data["threads"][0]
        # 3 human + 3 AI
        assert len(thread["messages"]) == 6

    def test_multi_thread_export_format(self, tmp_path):
        """Export multiple threads and verify each thread is independently valid."""
        saver = InMemorySaver()
        graph = create_simple_graph(saver)

        for i in range(3):
            graph.invoke(
                {"messages": [HumanMessage(content=f"Question {i}")]},
                {"configurable": {"thread_id": f"t{i}"}},
            )

        export = LangGraphExporter.from_memory(saver).export()
        export_file = tmp_path / "multi_threads.json"
        export.save(str(export_file))

        with open(export_file) as fh:
            data = json.load(fh)

        _assert_valid_export(data)
        assert len(data["threads"]) == 3
        thread_ids = {t["thread_id"] for t in data["threads"]}
        assert thread_ids == {"t0", "t1", "t2"}

    def test_tool_calls_in_export(self, tmp_path):
        """Export tool call + tool result and verify schema compliance."""
        saver = InMemorySaver()
        graph = create_tool_use_graph(saver)

        graph.invoke(
            {"messages": [HumanMessage(content="Search for Python tutorials")]},
            {"configurable": {"thread_id": "tool-thread"}},
        )

        export = LangGraphExporter.from_memory(saver).export()
        export_file = tmp_path / "tool.json"
        export.save(str(export_file))

        with open(export_file) as fh:
            data = json.load(fh)

        _assert_valid_export(data)
        thread = data["threads"][0]

        ai_msgs = [m for m in thread["messages"] if m["role"] == "assistant"]
        tool_msgs = [m for m in thread["messages"] if m["role"] == "tool"]

        assert len(ai_msgs) == 1
        assert len(tool_msgs) == 1

        ai = ai_msgs[0]
        assert "tool_calls" in ai
        assert ai["tool_calls"][0]["name"] == "search"
        assert ai["tool_calls"][0]["input"] == {"query": "Search for Python tutorials"}

        tool = tool_msgs[0]
        assert tool["tool_call_id"] == ai["tool_calls"][0]["id"]
        assert tool["tool_name"] == "search"

    def test_export_json_is_valid_json(self, tmp_path):
        """Ensure the exported file is always valid JSON."""
        saver = InMemorySaver()
        graph = create_simple_graph(saver)
        graph.invoke(
            {"messages": [HumanMessage(content="json validity")]},
            {"configurable": {"thread_id": "json-valid"}},
        )

        export = LangGraphExporter.from_memory(saver).export()
        export_file = tmp_path / "valid.json"
        export.save(str(export_file))

        # json.load raises if invalid
        with open(export_file) as fh:
            data = json.load(fh)
        assert data is not None

    def test_model_info_in_export(self, tmp_path):
        """Model name and token counts should appear in the exported JSON."""
        saver = InMemorySaver()
        graph = create_simple_graph(saver)
        graph.invoke(
            {"messages": [HumanMessage(content="token test")]},
            {"configurable": {"thread_id": "token-thread"}},
        )

        export = LangGraphExporter.from_memory(saver).export()
        export_file = tmp_path / "tokens.json"
        export.save(str(export_file))

        with open(export_file) as fh:
            data = json.load(fh)

        ai_msg = next(
            m for m in data["threads"][0]["messages"] if m["role"] == "assistant"
        )
        assert ai_msg.get("model") == "test-model"
        assert ai_msg.get("tokens_in") == 10
        assert ai_msg.get("tokens_out") == 20

    def test_thread_sorted_by_id(self, tmp_path):
        """Threads should be sorted by thread_id in the output."""
        saver = InMemorySaver()
        graph = create_simple_graph(saver)

        # Deliberately create out of order
        for name in ["z-thread", "a-thread", "m-thread"]:
            graph.invoke(
                {"messages": [HumanMessage(content="hi")]},
                {"configurable": {"thread_id": name}},
            )

        export = LangGraphExporter.from_memory(saver).export()
        ids = [t.thread_id for t in export.threads]
        assert ids == sorted(ids), "threads should be sorted by thread_id"

    def test_empty_saver_produces_empty_export(self, tmp_path):
        saver = InMemorySaver()
        export = LangGraphExporter.from_memory(saver).export()
        export_file = tmp_path / "empty.json"
        export.save(str(export_file))

        with open(export_file) as fh:
            data = json.load(fh)

        assert data["version"] == "1"
        assert data["threads"] == []


# ---------------------------------------------------------------------------
# CLI smoke test
# ---------------------------------------------------------------------------

class TestCLI:
    def test_cli_sqlite_export(self, tmp_path):
        """Smoke-test the CLI entry point with a SQLite database."""
        from langgraph.checkpoint.sqlite import SqliteSaver

        db_path = str(tmp_path / "cli_test.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="CLI test")]},
                {"configurable": {"thread_id": "cli-thread"}},
            )

        out_file = str(tmp_path / "cli_out.json")

        # Run via Python module so we don't need the entry point installed
        result = subprocess.run(
            [
                "python",
                "-m",
                "langgraph_export.cli",
                "--sqlite",
                db_path,
                "--output",
                out_file,
            ],
            capture_output=True,
            text=True,
            cwd=str(Path(__file__).parent.parent),
        )

        assert result.returncode == 0, (
            f"CLI failed:\nstdout: {result.stdout}\nstderr: {result.stderr}"
        )
        assert os.path.exists(out_file)

        with open(out_file) as fh:
            data = json.load(fh)

        _assert_valid_export(data)
        assert len(data["threads"]) == 1
        assert data["threads"][0]["thread_id"] == "cli-thread"

    def test_cli_stdout_export(self, tmp_path, capsys):
        """CLI with no --output should print JSON to stdout."""
        from langgraph.checkpoint.sqlite import SqliteSaver

        db_path = str(tmp_path / "stdout_test.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="stdout test")]},
                {"configurable": {"thread_id": "stdout-thread"}},
            )

        result = subprocess.run(
            [
                "python",
                "-m",
                "langgraph_export.cli",
                "--sqlite",
                db_path,
            ],
            capture_output=True,
            text=True,
            cwd=str(Path(__file__).parent.parent),
        )

        assert result.returncode == 0, (
            f"CLI failed:\nstdout: {result.stdout}\nstderr: {result.stderr}"
        )
        data = json.loads(result.stdout)
        _assert_valid_export(data)
