"""
Tests for exporting conversation data from LangGraph SqliteSaver.
"""
from __future__ import annotations

import json

import pytest
from langchain_core.messages import HumanMessage
from langgraph.checkpoint.sqlite import SqliteSaver

from langgraph_export import LangGraphExporter
from langgraph_export.backends.sqlite import SqliteBackend

from conftest import create_simple_graph, create_tool_use_graph


# ---------------------------------------------------------------------------
# Basic export
# ---------------------------------------------------------------------------

class TestExportFromSqlite:
    def test_export_single_thread(self, tmp_path):
        db_path = str(tmp_path / "test.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="Hello SQLite")]},
                {"configurable": {"thread_id": "sqlite-1"}},
            )

        exporter = LangGraphExporter.from_sqlite(db_path)
        export = exporter.export()

        assert export.version == "1"
        assert export.source_type == "langgraph"
        assert len(export.threads) == 1
        assert export.threads[0].thread_id == "sqlite-1"

    def test_export_multiple_threads(self, tmp_path):
        db_path = str(tmp_path / "multi.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            for i in range(3):
                graph.invoke(
                    {"messages": [HumanMessage(content=f"Thread {i}")]},
                    {"configurable": {"thread_id": f"thread-{i}"}},
                )

        exporter = LangGraphExporter.from_sqlite(db_path)
        export = exporter.export()

        assert len(export.threads) == 3
        ids = {t.thread_id for t in export.threads}
        assert ids == {"thread-0", "thread-1", "thread-2"}

    def test_export_empty_database(self, tmp_path):
        db_path = str(tmp_path / "empty.db")
        # Create DB but run no graphs so no checkpoints table exists
        exporter = LangGraphExporter.from_sqlite(db_path)
        export = exporter.export()
        assert len(export.threads) == 0

    def test_message_count_per_thread(self, tmp_path):
        db_path = str(tmp_path / "msgs.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="hi")]},
                {"configurable": {"thread_id": "t1"}},
            )

        export = LangGraphExporter.from_sqlite(db_path).export()
        thread = export.threads[0]
        # 1 human + 1 AI
        assert len(thread.messages) == 2

    def test_message_roles(self, tmp_path):
        db_path = str(tmp_path / "roles.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="Role test")]},
                {"configurable": {"thread_id": "roles"}},
            )

        export = LangGraphExporter.from_sqlite(db_path).export()
        thread = export.threads[0]
        assert thread.messages[0].role == "user"
        assert thread.messages[1].role == "assistant"

    def test_model_and_tokens_extracted(self, tmp_path):
        db_path = str(tmp_path / "meta.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="tokens")]},
                {"configurable": {"thread_id": "tok-test"}},
            )

        export = LangGraphExporter.from_sqlite(db_path).export()
        ai_msg = next(
            m for m in export.threads[0].messages if m.role == "assistant"
        )
        assert ai_msg.model == "test-model"
        assert ai_msg.tokens_in == 10
        assert ai_msg.tokens_out == 20


# ---------------------------------------------------------------------------
# Multi-turn
# ---------------------------------------------------------------------------

class TestSqliteMultiTurn:
    def test_multi_turn_accumulates_messages(self, tmp_path):
        db_path = str(tmp_path / "multi.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            cfg = {"configurable": {"thread_id": "mt"}}
            graph.invoke({"messages": [HumanMessage(content="First")]}, cfg)
            graph.invoke({"messages": [HumanMessage(content="Second")]}, cfg)

        export = LangGraphExporter.from_sqlite(db_path).export()
        assert len(export.threads) == 1
        # 2 human + 2 AI
        assert len(export.threads[0].messages) == 4

    def test_latest_checkpoint_used(self, tmp_path):
        """
        Export should reflect the final state, not an intermediate checkpoint.
        """
        db_path = str(tmp_path / "latest.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            cfg = {"configurable": {"thread_id": "lc"}}
            graph.invoke({"messages": [HumanMessage(content="A")]}, cfg)
            graph.invoke({"messages": [HumanMessage(content="B")]}, cfg)
            graph.invoke({"messages": [HumanMessage(content="C")]}, cfg)

        export = LangGraphExporter.from_sqlite(db_path).export()
        # 3 human + 3 AI
        assert len(export.threads[0].messages) == 6


# ---------------------------------------------------------------------------
# Tool calls
# ---------------------------------------------------------------------------

class TestSqliteToolCalls:
    def test_tool_calls_exported(self, tmp_path):
        db_path = str(tmp_path / "tools.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_tool_use_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="search for cats")]},
                {"configurable": {"thread_id": "tool-sqlite"}},
            )

        export = LangGraphExporter.from_sqlite(db_path).export()
        thread = export.threads[0]

        ai_msg = next(m for m in thread.messages if m.role == "assistant")
        assert len(ai_msg.tool_calls) == 1
        assert ai_msg.tool_calls[0].name == "search"
        assert ai_msg.tool_calls[0].input == {"query": "search for cats"}

    def test_tool_message_exported(self, tmp_path):
        db_path = str(tmp_path / "toolmsg.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_tool_use_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="find dogs")]},
                {"configurable": {"thread_id": "toolmsg-sqlite"}},
            )

        export = LangGraphExporter.from_sqlite(db_path).export()
        thread = export.threads[0]

        tool_msgs = [m for m in thread.messages if m.role == "tool"]
        assert len(tool_msgs) == 1
        assert tool_msgs[0].tool_name == "search"


# ---------------------------------------------------------------------------
# Serialisation
# ---------------------------------------------------------------------------

class TestSqliteSerialisation:
    def test_save_and_reload(self, tmp_path):
        db_path = str(tmp_path / "serial.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            graph.invoke(
                {"messages": [HumanMessage(content="serialise")]},
                {"configurable": {"thread_id": "ser-1"}},
            )

        export = LangGraphExporter.from_sqlite(db_path).export()
        out_file = str(tmp_path / "out.json")
        export.save(out_file)

        with open(out_file) as fh:
            data = json.load(fh)

        assert data["version"] == "1"
        assert data["source_type"] == "langgraph"
        assert len(data["threads"]) == 1
        assert data["threads"][0]["thread_id"] == "ser-1"
        assert len(data["threads"][0]["messages"]) == 2


# ---------------------------------------------------------------------------
# SqliteBackend directly
# ---------------------------------------------------------------------------

class TestSqliteBackend:
    def test_thread_ids(self, tmp_path):
        db_path = str(tmp_path / "backend.db")
        with SqliteSaver.from_conn_string(db_path) as saver:
            graph = create_simple_graph(saver)
            for i in range(2):
                graph.invoke(
                    {"messages": [HumanMessage(content=f"msg {i}")]},
                    {"configurable": {"thread_id": f"bt-{i}"}},
                )

        backend = SqliteBackend(db_path)
        ids = backend.thread_ids()
        assert set(ids) == {"bt-0", "bt-1"}

    def test_empty_db_thread_ids(self, tmp_path):
        db_path = str(tmp_path / "empty_backend.db")
        backend = SqliteBackend(db_path)
        assert backend.thread_ids() == []
