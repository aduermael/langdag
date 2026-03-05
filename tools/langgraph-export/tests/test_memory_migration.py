"""
Tests for exporting conversation data from LangGraph InMemorySaver.
"""
from __future__ import annotations

import pytest
from langchain_core.messages import HumanMessage
from langgraph.checkpoint.memory import InMemorySaver

from langgraph_export import LangGraphExporter
from langgraph_export.types import ExportData, ExportThread

from conftest import create_simple_graph, create_tool_use_graph


# ---------------------------------------------------------------------------
# Basic export tests
# ---------------------------------------------------------------------------

class TestExportFromMemory:
    def test_export_single_thread(self, simple_graph):
        graph, saver = simple_graph
        config = {"configurable": {"thread_id": "thread-single"}}
        graph.invoke({"messages": [HumanMessage(content="Hello")]}, config)

        exporter = LangGraphExporter.from_memory(saver)
        export = exporter.export()

        assert isinstance(export, ExportData)
        assert export.version == "1"
        assert export.source_type == "langgraph"
        assert len(export.threads) == 1

    def test_export_multiple_threads(self, memory_saver):
        graph = create_simple_graph(memory_saver)
        configs = [
            {"configurable": {"thread_id": "thread-1"}},
            {"configurable": {"thread_id": "thread-2"}},
            {"configurable": {"thread_id": "thread-3"}},
        ]
        for i, cfg in enumerate(configs):
            graph.invoke({"messages": [HumanMessage(content=f"Message {i}")]}, cfg)

        exporter = LangGraphExporter.from_memory(memory_saver)
        export = exporter.export()

        assert len(export.threads) == 3
        thread_ids = {t.thread_id for t in export.threads}
        assert thread_ids == {"thread-1", "thread-2", "thread-3"}

    def test_export_empty_saver(self):
        saver = InMemorySaver()
        exporter = LangGraphExporter.from_memory(saver)
        export = exporter.export()
        assert len(export.threads) == 0

    def test_export_data_has_exported_at(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="hi")]},
            {"configurable": {"thread_id": "t1"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        assert export.exported_at.endswith("Z")


# ---------------------------------------------------------------------------
# Message content & structure
# ---------------------------------------------------------------------------

class TestMessageStructure:
    def test_message_roles(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="Hello")]},
            {"configurable": {"thread_id": "roles-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        thread = export.threads[0]

        assert len(thread.messages) == 2
        assert thread.messages[0].role == "user"
        assert thread.messages[1].role == "assistant"

    def test_user_message_content(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="What is the meaning of life?")]},
            {"configurable": {"thread_id": "content-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        thread = export.threads[0]
        assert thread.messages[0].content == "What is the meaning of life?"

    def test_assistant_message_content(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="Hello")]},
            {"configurable": {"thread_id": "ai-content"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        thread = export.threads[0]
        assert "Response to: Hello" in thread.messages[1].content

    def test_message_ids_are_populated(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="Ping")]},
            {"configurable": {"thread_id": "ids-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        for msg in export.threads[0].messages:
            assert msg.id, f"Message id should not be empty: {msg}"

    def test_model_extracted_from_ai_message(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="Hi")]},
            {"configurable": {"thread_id": "model-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        ai_msg = next(m for m in export.threads[0].messages if m.role == "assistant")
        assert ai_msg.model == "test-model"

    def test_token_counts_extracted(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="Tokens please")]},
            {"configurable": {"thread_id": "tokens-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        ai_msg = next(m for m in export.threads[0].messages if m.role == "assistant")
        assert ai_msg.tokens_in == 10
        assert ai_msg.tokens_out == 20


# ---------------------------------------------------------------------------
# Multi-turn conversations
# ---------------------------------------------------------------------------

class TestMultiTurnConversation:
    def test_multi_turn_message_count(self, memory_saver):
        """
        Two user turns in the same thread should yield 4 messages total
        (2 human + 2 AI) because LangGraph accumulates messages.
        """
        graph = create_simple_graph(memory_saver)
        config = {"configurable": {"thread_id": "multi-turn"}}

        graph.invoke({"messages": [HumanMessage(content="Hello")]}, config)
        graph.invoke({"messages": [HumanMessage(content="Tell me more")]}, config)

        export = LangGraphExporter.from_memory(memory_saver).export()
        assert len(export.threads) == 1
        thread = export.threads[0]
        assert len(thread.messages) == 4

    def test_multi_turn_message_order(self, memory_saver):
        graph = create_simple_graph(memory_saver)
        config = {"configurable": {"thread_id": "order-test"}}

        graph.invoke({"messages": [HumanMessage(content="First")]}, config)
        graph.invoke({"messages": [HumanMessage(content="Second")]}, config)

        export = LangGraphExporter.from_memory(memory_saver).export()
        thread = export.threads[0]
        roles = [m.role for m in thread.messages]
        assert roles == ["user", "assistant", "user", "assistant"]

    def test_two_threads_independence(self, memory_saver):
        """
        Continuing thread-1 should not affect thread-2.
        """
        graph = create_simple_graph(memory_saver)
        cfg1 = {"configurable": {"thread_id": "thread-1"}}
        cfg2 = {"configurable": {"thread_id": "thread-2"}}

        graph.invoke({"messages": [HumanMessage(content="Hello")]}, cfg1)
        graph.invoke({"messages": [HumanMessage(content="How are you?")]}, cfg2)
        # Continue thread 1
        graph.invoke({"messages": [HumanMessage(content="Tell me more")]}, cfg1)

        export = LangGraphExporter.from_memory(memory_saver).export()
        assert len(export.threads) == 2

        t1 = next(t for t in export.threads if t.thread_id == "thread-1")
        t2 = next(t for t in export.threads if t.thread_id == "thread-2")
        assert len(t1.messages) == 4  # 2 user + 2 assistant
        assert len(t2.messages) == 2  # 1 user + 1 assistant


# ---------------------------------------------------------------------------
# Tool calls
# ---------------------------------------------------------------------------

class TestToolCalls:
    def test_tool_call_present(self, memory_saver):
        graph = create_tool_use_graph(memory_saver)
        graph.invoke(
            {"messages": [HumanMessage(content="find Python docs")]},
            {"configurable": {"thread_id": "tool-test"}},
        )
        export = LangGraphExporter.from_memory(memory_saver).export()
        thread = export.threads[0]

        ai_msg = next(m for m in thread.messages if m.role == "assistant")
        assert len(ai_msg.tool_calls) == 1
        tc = ai_msg.tool_calls[0]
        assert tc.name == "search"
        assert tc.input == {"query": "find Python docs"}

    def test_tool_message_present(self, memory_saver):
        graph = create_tool_use_graph(memory_saver)
        graph.invoke(
            {"messages": [HumanMessage(content="search me")]},
            {"configurable": {"thread_id": "tool-msg-test"}},
        )
        export = LangGraphExporter.from_memory(memory_saver).export()
        thread = export.threads[0]

        tool_msgs = [m for m in thread.messages if m.role == "tool"]
        assert len(tool_msgs) == 1
        assert tool_msgs[0].tool_name == "search"
        assert tool_msgs[0].tool_call_id == "call_test_001"

    def test_tool_call_id_matches(self, memory_saver):
        graph = create_tool_use_graph(memory_saver)
        graph.invoke(
            {"messages": [HumanMessage(content="search")]},
            {"configurable": {"thread_id": "tool-id-test"}},
        )
        export = LangGraphExporter.from_memory(memory_saver).export()
        thread = export.threads[0]

        ai_msg = next(m for m in thread.messages if m.role == "assistant")
        tool_msg = next(m for m in thread.messages if m.role == "tool")
        assert ai_msg.tool_calls[0].id == tool_msg.tool_call_id


# ---------------------------------------------------------------------------
# Serialisation
# ---------------------------------------------------------------------------

class TestSerialisation:
    def test_to_dict_structure(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="hi")]},
            {"configurable": {"thread_id": "serial-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        d = export.to_dict()

        assert d["version"] == "1"
        assert d["source_type"] == "langgraph"
        assert "exported_at" in d
        assert isinstance(d["threads"], list)
        assert len(d["threads"]) == 1

    def test_to_json_is_valid(self, simple_graph):
        import json

        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="json test")]},
            {"configurable": {"thread_id": "json-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        parsed = json.loads(export.to_json())
        assert parsed["version"] == "1"

    def test_save_to_file(self, simple_graph, tmp_path):
        import json

        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="save test")]},
            {"configurable": {"thread_id": "save-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        out_file = str(tmp_path / "export.json")
        export.save(out_file)

        with open(out_file) as fh:
            data = json.load(fh)
        assert data["version"] == "1"
        assert len(data["threads"]) == 1

    def test_thread_metadata_original_id(self, simple_graph):
        graph, saver = simple_graph
        graph.invoke(
            {"messages": [HumanMessage(content="meta")]},
            {"configurable": {"thread_id": "meta-test"}},
        )
        export = LangGraphExporter.from_memory(saver).export()
        thread = export.threads[0]
        assert thread.metadata.get("original_thread_id") == "meta-test"
