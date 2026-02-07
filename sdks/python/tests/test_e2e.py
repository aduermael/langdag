"""E2E tests that connect to a running LangDAG server with mock provider.

Run with: LANGDAG_E2E_URL=http://localhost:8080 pytest tests/test_e2e.py -v
The server must be started with LANGDAG_PROVIDER=mock.
"""

import os

import pytest

from langdag.client import LangDAGClient
from langdag.async_client import AsyncLangDAGClient
from langdag.models import ChatResponse, SSEEventType


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

    def test_chat_non_streaming(self):
        with self.get_client() as client:
            resp = client.chat("Hello, this is a test")
            assert isinstance(resp, ChatResponse)
            assert resp.dag_id
            assert resp.node_id
            assert resp.content

            # Continue
            resp2 = client.continue_chat(resp.dag_id, "Follow up")
            assert isinstance(resp2, ChatResponse)
            assert resp2.dag_id == resp.dag_id
            assert resp2.content

            # Get DAG
            dag = client.get_dag(resp.dag_id)
            assert dag.id == resp.dag_id
            assert len(dag.nodes) >= 4

            # List DAGs
            dags = client.list_dags()
            dag_ids = [d.id for d in dags]
            assert resp.dag_id in dag_ids

            # Delete
            del_resp = client.delete_dag(resp.dag_id)
            assert del_resp["status"] == "deleted"

    def test_chat_streaming(self):
        with self.get_client() as client:
            events = list(client.chat("Tell me something", stream=True))
            assert len(events) > 0

            event_types = [e.event for e in events]
            assert SSEEventType.START in event_types
            assert SSEEventType.DELTA in event_types
            assert SSEEventType.DONE in event_types

            # Check content was streamed
            content_parts = [e.content for e in events if e.content]
            assert len(content_parts) > 0

            # Get dag_id from done event and clean up
            done_events = [e for e in events if e.event == SSEEventType.DONE]
            if done_events and done_events[0].dag_id:
                client.delete_dag(done_events[0].dag_id)

    def test_fork_chat(self):
        with self.get_client() as client:
            resp = client.chat("First message")
            assert isinstance(resp, ChatResponse)

            fork_resp = client.fork_chat(
                resp.dag_id,
                resp.node_id,
                "Alternative path",
            )
            assert isinstance(fork_resp, ChatResponse)
            assert fork_resp.content

            # Clean up
            client.delete_dag(resp.dag_id)
            if fork_resp.dag_id != resp.dag_id:
                client.delete_dag(fork_resp.dag_id)


class TestE2EAsync:
    def get_client(self) -> AsyncLangDAGClient:
        return AsyncLangDAGClient(base_url=E2E_URL, timeout=30.0)

    async def test_health(self):
        async with self.get_client() as client:
            result = await client.health()
            assert result["status"] == "ok"

    async def test_chat_non_streaming(self):
        async with self.get_client() as client:
            resp = await client.chat("Hello from async")
            assert isinstance(resp, ChatResponse)
            assert resp.dag_id
            assert resp.content

            # Continue
            resp2 = await client.continue_chat(resp.dag_id, "Async follow up")
            assert isinstance(resp2, ChatResponse)
            assert resp2.content

            # Clean up
            await client.delete_dag(resp.dag_id)

    async def test_chat_streaming(self):
        async with self.get_client() as client:
            events = []
            async for event in await client.chat("Stream test", stream=True):
                events.append(event)

            assert len(events) > 0
            event_types = [e.event for e in events]
            assert SSEEventType.START in event_types
            assert SSEEventType.DELTA in event_types
            assert SSEEventType.DONE in event_types

            # Clean up
            done_events = [e for e in events if e.event == SSEEventType.DONE]
            if done_events and done_events[0].dag_id:
                await client.delete_dag(done_events[0].dag_id)
