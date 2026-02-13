"""Unit tests for the asynchronous LangDAG client."""

import json

import pytest
from pytest_httpx import HTTPXMock

from langdag.async_client import AsyncLangDAGClient
from langdag.exceptions import (
    APIError,
    AuthenticationError,
    BadRequestError,
    ConnectionError,
    NotFoundError,
)
from langdag.models import PromptResponse, SSEEventType


class TestAsyncClientInit:
    def test_default_base_url(self):
        client = AsyncLangDAGClient()
        assert client.base_url == "http://localhost:8080"

    def test_trailing_slash_stripped(self):
        client = AsyncLangDAGClient(base_url="http://localhost:8080/")
        assert client.base_url == "http://localhost:8080"

    async def test_context_manager(self):
        async with AsyncLangDAGClient() as client:
            assert client._client is None
        assert client._client is None


class TestAsyncHealth:
    async def test_health_ok(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "ok"})
        async with AsyncLangDAGClient() as client:
            result = await client.health()
            assert result["status"] == "ok"


class TestAsyncErrorHandling:
    async def test_authentication_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=401,
            json={"error": "unauthorized"},
        )
        async with AsyncLangDAGClient() as client:
            with pytest.raises(AuthenticationError):
                await client.health()

    async def test_not_found_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=404,
            json={"error": "not found"},
        )
        async with AsyncLangDAGClient() as client:
            with pytest.raises(NotFoundError):
                await client.get_node("nonexistent")

    async def test_generic_api_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=500,
            json={"error": "internal server error"},
        )
        async with AsyncLangDAGClient() as client:
            with pytest.raises(APIError):
                await client.health()

    async def test_connection_error(self):
        async with AsyncLangDAGClient(base_url="http://localhost:1") as client:
            with pytest.raises(ConnectionError):
                await client.health()


class TestAsyncListRoots:
    async def test_list_roots(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json=[
                {
                    "id": "node-1",
                    "sequence": 0,
                    "node_type": "user",
                    "content": "Hello",
                    "created_at": "2024-01-01T00:00:00Z",
                    "title": "First conversation",
                },
            ]
        )
        async with AsyncLangDAGClient() as client:
            roots = await client.list_roots()
            assert len(roots) == 1
            assert roots[0].id == "node-1"
            assert roots[0].title == "First conversation"


class TestAsyncGetNode:
    async def test_get_node(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "id": "node-1",
                "sequence": 0,
                "node_type": "user",
                "content": "Hello",
                "created_at": "2024-01-01T00:00:00Z",
                "title": "My conversation",
            }
        )
        async with AsyncLangDAGClient() as client:
            node = await client.get_node("node-1")
            assert node.id == "node-1"
            assert node.content == "Hello"
            assert node.title == "My conversation"


class TestAsyncGetTree:
    async def test_get_tree(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json=[
                {
                    "id": "node-1",
                    "sequence": 0,
                    "node_type": "user",
                    "content": "Hello",
                    "created_at": "2024-01-01T00:00:00Z",
                },
                {
                    "id": "node-2",
                    "parent_id": "node-1",
                    "sequence": 1,
                    "node_type": "assistant",
                    "content": "Hi there!",
                    "created_at": "2024-01-01T00:00:01Z",
                },
            ]
        )
        async with AsyncLangDAGClient() as client:
            tree = await client.get_tree("node-1")
            assert len(tree) == 2
            assert tree[1].parent_id == "node-1"


class TestAsyncPrompt:
    async def test_prompt_non_streaming(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-456",
                "content": "Hello back!",
                "tokens_in": 5,
                "tokens_out": 3,
            }
        )
        async with AsyncLangDAGClient() as client:
            resp = await client.prompt("Hello")
            assert isinstance(resp, PromptResponse)
            assert resp.node_id == "node-456"
            assert resp.content == "Hello back!"

    async def test_prompt_sends_correct_body(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-1",
                "content": "ok",
            }
        )
        async with AsyncLangDAGClient() as client:
            await client.prompt("Hello", model="test-model")
        request = httpx_mock.get_request()
        body = json.loads(request.content)
        assert body["message"] == "Hello"
        assert body["model"] == "test-model"


class TestAsyncPromptFrom:
    async def test_prompt_from(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-789",
                "content": "Continued!",
            }
        )
        async with AsyncLangDAGClient() as client:
            resp = await client.prompt_from("node-123", "Follow up")
            assert isinstance(resp, PromptResponse)
            assert resp.content == "Continued!"


class TestAsyncDeleteNode:
    async def test_delete_node(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "deleted", "id": "node-1"})
        async with AsyncLangDAGClient() as client:
            resp = await client.delete_node("node-1")
            assert resp["status"] == "deleted"


class TestAsyncAPIKeyHeader:
    async def test_api_key_sent(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "ok"})
        async with AsyncLangDAGClient(api_key="my-key") as client:
            await client.health()
        request = httpx_mock.get_request()
        assert request.headers["X-API-Key"] == "my-key"


# --- 3b: Streaming unit tests for async client ---


class TestAsyncStreaming:
    async def test_prompt_stream_iteration(self, httpx_mock: HTTPXMock):
        sse_body = (
            "event: start\ndata: {}\n\n"
            'event: delta\ndata: {"content":"Hello "}\n\n'
            'event: delta\ndata: {"content":"world!"}\n\n'
            'event: done\ndata: {"node_id":"n-1"}\n\n'
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        async with AsyncLangDAGClient() as client:
            events = []
            stream = await client.prompt("Hello", stream=True)
            async for event in stream:
                events.append(event)
        assert len(events) == 4
        assert events[0].event == SSEEventType.START
        assert events[1].content == "Hello "
        assert events[2].content == "world!"
        assert events[3].event == SSEEventType.DONE
        assert events[3].node_id == "n-1"

    async def test_prompt_from_stream_iteration(self, httpx_mock: HTTPXMock):
        sse_body = (
            "event: start\ndata: {}\n\n"
            'event: delta\ndata: {"content":"Continued"}\n\n'
            'event: done\ndata: {"node_id":"n-2"}\n\n'
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        async with AsyncLangDAGClient() as client:
            events = []
            stream = await client.prompt_from("n-1", "More please", stream=True)
            async for event in stream:
                events.append(event)
        assert len(events) == 3
        assert events[2].node_id == "n-2"

    async def test_stream_error_event(self, httpx_mock: HTTPXMock):
        sse_body = (
            "event: start\ndata: {}\n\n"
            "event: error\ndata: something went wrong\n\n"
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        async with AsyncLangDAGClient() as client:
            events = []
            stream = await client.prompt("Hello", stream=True)
            async for event in stream:
                events.append(event)
        assert len(events) == 2
        assert events[1].event == SSEEventType.ERROR

    async def test_stream_http_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=500,
            json={"error": "server error"},
        )
        async with AsyncLangDAGClient() as client:
            with pytest.raises(APIError) as exc_info:
                stream = await client.prompt("Hello", stream=True)
                async for _ in stream:
                    pass
            assert exc_info.value.status_code == 500

    async def test_stream_collect_content(self, httpx_mock: HTTPXMock):
        sse_body = (
            "event: start\ndata: {}\n\n"
            'event: delta\ndata: {"content":"One "}\n\n'
            'event: delta\ndata: {"content":"two "}\n\n'
            'event: delta\ndata: {"content":"three"}\n\n'
            'event: done\ndata: {"node_id":"n-1"}\n\n'
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        async with AsyncLangDAGClient() as client:
            content = ""
            stream = await client.prompt("Hello", stream=True)
            async for event in stream:
                if event.content:
                    content += event.content
        assert content == "One two three"

    async def test_stream_non_json_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=502,
            content=b"<html>Bad Gateway</html>",
            headers={"content-type": "text/html"},
        )
        async with AsyncLangDAGClient() as client:
            with pytest.raises(APIError) as exc_info:
                stream = await client.prompt("Hello", stream=True)
                async for _ in stream:
                    pass
            assert exc_info.value.status_code == 502
