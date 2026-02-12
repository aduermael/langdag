"""E2E tests that connect to a running LangDAG server with mock provider.

Run with: LANGDAG_E2E_URL=http://localhost:8080 pytest tests/test_e2e.py -v
The server must be started with LANGDAG_PROVIDER=mock.
"""

import os

import pytest

from langdag.client import LangDAGClient
from langdag.async_client import AsyncLangDAGClient
from langdag.models import PromptResponse, SSEEventType


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
