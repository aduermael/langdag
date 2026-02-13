"""E2E tests that connect to a running LangDAG server with mock provider.

Run with: LANGDAG_E2E_URL=http://localhost:8080 pytest tests/test_e2e.py -v
The server must be started with LANGDAG_PROVIDER=mock.
"""

import os

import pytest

from datetime import datetime

from langdag.client import LangDAGClient
from langdag.async_client import AsyncLangDAGClient
from langdag.exceptions import NotFoundError
from langdag.models import NodeType, PromptResponse, SSEEventType


E2E_URL = os.environ.get("LANGDAG_E2E_URL")

pytestmark = pytest.mark.skipif(
    E2E_URL is None,
    reason="LANGDAG_E2E_URL not set, skipping E2E tests",
)


class TestE2ESync:
    def get_client(self) -> LangDAGClient:
        return LangDAGClient(base_url=E2E_URL, timeout=30.0)

    def test_health(self):
        with self.get_client() as client:
            result = client.health()
            assert result["status"] == "ok"

    def test_prompt_non_streaming(self):
        with self.get_client() as client:
            # Start a new conversation
            resp = client.prompt("Hello, this is a test")
            assert isinstance(resp, PromptResponse)
            assert resp.node_id
            assert resp.content

            # Continue from the response node
            resp2 = client.prompt_from(resp.node_id, "Follow up")
            assert isinstance(resp2, PromptResponse)
            assert resp2.node_id
            assert resp2.content

            # Get the tree to see all nodes
            tree = client.get_tree(resp.node_id)
            assert len(tree) >= 2

            # List root nodes
            roots = client.list_roots()
            root_ids = [n.id for n in roots]
            # The root should be in the list (it's the first user node)
            # Find the root of our conversation
            root_node = None
            for node in tree:
                if node.parent_id is None:
                    root_node = node
                    break
            assert root_node is not None
            assert root_node.id in root_ids

            # Delete the root node (cleans up entire tree)
            del_resp = client.delete_node(root_node.id)
            assert del_resp["status"] == "deleted"

    def test_prompt_streaming(self):
        with self.get_client() as client:
            events = list(client.prompt("Tell me something", stream=True))
            assert len(events) > 0

            event_types = [e.event for e in events]
            assert SSEEventType.START in event_types
            assert SSEEventType.DELTA in event_types
            assert SSEEventType.DONE in event_types

            # Check content was streamed
            content_parts = [e.content for e in events if e.content]
            assert len(content_parts) > 0

            # Get node_id from done event and clean up
            done_events = [e for e in events if e.event == SSEEventType.DONE]
            if done_events and done_events[0].node_id:
                # Find the root and delete it
                node_id = done_events[0].node_id
                tree = client.get_tree(node_id)
                for node in tree:
                    if node.parent_id is None:
                        client.delete_node(node.id)
                        break

    def test_prompt_from_branching(self):
        with self.get_client() as client:
            # Start conversation
            resp = client.prompt("First message")
            assert isinstance(resp, PromptResponse)

            # Branch from the response node with a different question
            branch_resp = client.prompt_from(
                resp.node_id,
                "Alternative path",
            )
            assert isinstance(branch_resp, PromptResponse)
            assert branch_resp.content

            # Clean up - find and delete the root
            tree = client.get_tree(resp.node_id)
            for node in tree:
                if node.parent_id is None:
                    client.delete_node(node.id)
                    break

    def test_prompt_from_streaming(self):
        with self.get_client() as client:
            # Create a non-streaming conversation first
            resp = client.prompt("First message")
            assert isinstance(resp, PromptResponse)
            assert resp.node_id

            # Continue with streaming from the response node
            events = list(
                client.prompt_from(resp.node_id, "Continue streaming", stream=True)
            )
            assert len(events) > 0

            event_types = [e.event for e in events]
            assert SSEEventType.START in event_types
            assert SSEEventType.DELTA in event_types
            assert SSEEventType.DONE in event_types

            # Get node_id from done event and clean up
            done_events = [e for e in events if e.event == SSEEventType.DONE]
            assert len(done_events) > 0
            node_id = done_events[0].node_id
            assert node_id

            tree = client.get_tree(node_id)
            for node in tree:
                if node.parent_id is None:
                    client.delete_node(node.id)
                    break

    def test_get_nonexistent_node(self):
        with self.get_client() as client:
            with pytest.raises(NotFoundError) as exc_info:
                client.get_node("nonexistent-node-id-12345")
            assert exc_info.value.status_code == 404

    def test_delete_nonexistent_node(self):
        with self.get_client() as client:
            with pytest.raises(NotFoundError):
                client.delete_node("nonexistent-node-id-12345")

    def test_node_field_parsing(self):
        with self.get_client() as client:
            resp = client.prompt("Test field parsing")
            assert isinstance(resp, PromptResponse)

            tree = client.get_tree(resp.node_id)
            assert len(tree) >= 2

            # Find the user and assistant nodes
            user_node = None
            assistant_node = None
            for node in tree:
                if node.node_type == NodeType.USER:
                    user_node = node
                elif node.node_type == NodeType.ASSISTANT:
                    assistant_node = node

            # Verify user node fields
            assert user_node is not None
            assert user_node.content
            assert user_node.node_type == NodeType.USER
            assert user_node.sequence >= 0
            assert isinstance(user_node.created_at, datetime)
            assert user_node.id

            # Verify assistant node fields
            assert assistant_node is not None
            assert assistant_node.content
            assert assistant_node.node_type == NodeType.ASSISTANT
            assert assistant_node.parent_id is not None

            # Verify tokens are present (mock provider sets them)
            assert assistant_node.tokens_in is not None
            assert assistant_node.tokens_out is not None

            # Clean up - find and delete the root
            for node in tree:
                if node.parent_id is None:
                    client.delete_node(node.id)
                    break


class TestE2EAsync:
    def get_client(self) -> AsyncLangDAGClient:
        return AsyncLangDAGClient(base_url=E2E_URL, timeout=30.0)

    async def test_health(self):
        async with self.get_client() as client:
            result = await client.health()
            assert result["status"] == "ok"

    async def test_prompt_non_streaming(self):
        async with self.get_client() as client:
            resp = await client.prompt("Hello from async")
            assert isinstance(resp, PromptResponse)
            assert resp.node_id
            assert resp.content

            # Continue from the response node
            resp2 = await client.prompt_from(resp.node_id, "Async follow up")
            assert isinstance(resp2, PromptResponse)
            assert resp2.content

            # Clean up - find and delete the root
            tree = await client.get_tree(resp.node_id)
            for node in tree:
                if node.parent_id is None:
                    await client.delete_node(node.id)
                    break

    async def test_prompt_streaming(self):
        async with self.get_client() as client:
            events = []
            async for event in client.prompt("Stream test", stream=True):
                events.append(event)

            assert len(events) > 0
            event_types = [e.event for e in events]
            assert SSEEventType.START in event_types
            assert SSEEventType.DELTA in event_types
            assert SSEEventType.DONE in event_types

            # Clean up
            done_events = [e for e in events if e.event == SSEEventType.DONE]
            if done_events and done_events[0].node_id:
                node_id = done_events[0].node_id
                tree = await client.get_tree(node_id)
                for node in tree:
                    if node.parent_id is None:
                        await client.delete_node(node.id)
                        break

    async def test_prompt_from_branching(self):
        async with self.get_client() as client:
            # Start conversation
            resp = await client.prompt("First message")
            assert isinstance(resp, PromptResponse)

            # Branch from the response node with a different question
            branch_resp = await client.prompt_from(
                resp.node_id,
                "Alternative path",
            )
            assert isinstance(branch_resp, PromptResponse)
            assert branch_resp.content

            # Clean up - find and delete the root
            tree = await client.get_tree(resp.node_id)
            for node in tree:
                if node.parent_id is None:
                    await client.delete_node(node.id)
                    break

    async def test_prompt_from_streaming(self):
        async with self.get_client() as client:
            # Create a non-streaming conversation first
            resp = await client.prompt("First message")
            assert isinstance(resp, PromptResponse)
            assert resp.node_id

            # Continue with streaming from the response node
            events = []
            async for event in client.prompt_from(
                resp.node_id, "Continue streaming", stream=True
            ):
                events.append(event)
            assert len(events) > 0

            event_types = [e.event for e in events]
            assert SSEEventType.START in event_types
            assert SSEEventType.DELTA in event_types
            assert SSEEventType.DONE in event_types

            # Get node_id from done event and clean up
            done_events = [e for e in events if e.event == SSEEventType.DONE]
            assert len(done_events) > 0
            node_id = done_events[0].node_id
            assert node_id

            tree = await client.get_tree(node_id)
            for node in tree:
                if node.parent_id is None:
                    await client.delete_node(node.id)
                    break

    async def test_get_nonexistent_node(self):
        async with self.get_client() as client:
            with pytest.raises(NotFoundError) as exc_info:
                await client.get_node("nonexistent-node-id-12345")
            assert exc_info.value.status_code == 404

    async def test_delete_nonexistent_node(self):
        async with self.get_client() as client:
            with pytest.raises(NotFoundError):
                await client.delete_node("nonexistent-node-id-12345")

    async def test_node_field_parsing(self):
        async with self.get_client() as client:
            resp = await client.prompt("Test field parsing")
            assert isinstance(resp, PromptResponse)

            tree = await client.get_tree(resp.node_id)
            assert len(tree) >= 2

            # Find the user and assistant nodes
            user_node = None
            assistant_node = None
            for node in tree:
                if node.node_type == NodeType.USER:
                    user_node = node
                elif node.node_type == NodeType.ASSISTANT:
                    assistant_node = node

            # Verify user node fields
            assert user_node is not None
            assert user_node.content
            assert user_node.node_type == NodeType.USER
            assert user_node.sequence >= 0
            assert isinstance(user_node.created_at, datetime)
            assert user_node.id

            # Verify assistant node fields
            assert assistant_node is not None
            assert assistant_node.content
            assert assistant_node.node_type == NodeType.ASSISTANT
            assert assistant_node.parent_id is not None

            # Verify tokens are present (mock provider sets them)
            assert assistant_node.tokens_in is not None
            assert assistant_node.tokens_out is not None

            # Clean up - find and delete the root
            for node in tree:
                if node.parent_id is None:
                    await client.delete_node(node.id)
                    break
