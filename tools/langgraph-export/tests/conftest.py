"""
Shared fixtures and helpers for the langgraph-export test suite.
"""
from __future__ import annotations

import operator
from typing import Annotated, TypedDict

import pytest
from langchain_core.messages import AIMessage, BaseMessage, HumanMessage, ToolMessage
from langgraph.checkpoint.memory import InMemorySaver
from langgraph.graph import END, START, StateGraph


# ---------------------------------------------------------------------------
# Graph state
# ---------------------------------------------------------------------------

class ConversationState(TypedDict):
    messages: Annotated[list[BaseMessage], operator.add]


# ---------------------------------------------------------------------------
# Graph factories
# ---------------------------------------------------------------------------

def create_simple_graph(checkpointer):
    """
    Create a minimal passthrough graph that echoes back a response to every
    human message.  The response includes usage_metadata and response_metadata
    so we can verify token / model extraction in tests.
    """

    def chatbot_node(state: ConversationState) -> ConversationState:
        # Find the last human message
        last_human = next(
            m for m in reversed(state["messages"]) if m.type == "human"
        )
        ai_msg = AIMessage(
            content=f"Response to: {last_human.content}",
            id="ai-test-id",
            usage_metadata={
                "input_tokens": 10,
                "output_tokens": 20,
                "total_tokens": 30,
            },
            response_metadata={"model_name": "test-model"},
        )
        return {"messages": [ai_msg]}

    graph = StateGraph(ConversationState)
    graph.add_node("chatbot", chatbot_node)
    graph.add_edge(START, "chatbot")
    graph.add_edge("chatbot", END)
    return graph.compile(checkpointer=checkpointer)


def create_tool_use_graph(checkpointer):
    """
    Create a graph that simulates an AI message with a tool call followed by
    a ToolMessage result.
    """

    def agent_node(state: ConversationState) -> ConversationState:
        last_human = next(
            m for m in reversed(state["messages"]) if m.type == "human"
        )
        tool_call_id = "call_test_001"
        ai_msg = AIMessage(
            content="",
            id="ai-tool-msg",
            tool_calls=[
                {
                    "id": tool_call_id,
                    "name": "search",
                    "args": {"query": last_human.content},
                    "type": "tool_call",
                }
            ],
            usage_metadata={"input_tokens": 5, "output_tokens": 3, "total_tokens": 8},
            response_metadata={"model_name": "tool-model"},
        )
        tool_msg = ToolMessage(
            content=f"Results for: {last_human.content}",
            tool_call_id=tool_call_id,
            name="search",
        )
        return {"messages": [ai_msg, tool_msg]}

    graph = StateGraph(ConversationState)
    graph.add_node("agent", agent_node)
    graph.add_edge(START, "agent")
    graph.add_edge("agent", END)
    return graph.compile(checkpointer=checkpointer)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def memory_saver():
    """A fresh InMemorySaver for each test."""
    return InMemorySaver()


@pytest.fixture
def simple_graph(memory_saver):
    """A compiled simple graph backed by InMemorySaver."""
    return create_simple_graph(memory_saver), memory_saver


@pytest.fixture
def tool_graph(memory_saver):
    """A compiled tool-use graph backed by InMemorySaver."""
    return create_tool_use_graph(memory_saver), memory_saver
