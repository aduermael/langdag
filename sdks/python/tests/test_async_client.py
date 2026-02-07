"""Unit tests for the asynchronous LangDAG client."""

import json

import pytest
from pytest_httpx import HTTPXMock

from langdag.async_client import AsyncLangDAGClient
from langdag.exceptions import (
    APIError,
    AuthenticationError,
    ConnectionError,
    NotFoundError,
)
from langdag.models import ChatResponse


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
                await client.get_dag("nonexistent")

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


class TestAsyncListDAGs:
    async def test_list_dags(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json=[
                {
                    "id": "dag-1",
                    "status": "completed",
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                },
            ]
        )
        async with AsyncLangDAGClient() as client:
            dags = await client.list_dags()
            assert len(dags) == 1
            assert dags[0].id == "dag-1"


class TestAsyncChat:
    async def test_chat_non_streaming(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "dag_id": "dag-123",
                "node_id": "node-456",
                "content": "Hello back!",
            }
        )
        async with AsyncLangDAGClient() as client:
            resp = await client.chat("Hello")
            assert isinstance(resp, ChatResponse)
            assert resp.dag_id == "dag-123"
            assert resp.content == "Hello back!"

    async def test_chat_sends_correct_body(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "dag_id": "dag-1",
                "node_id": "node-1",
                "content": "ok",
            }
        )
        async with AsyncLangDAGClient() as client:
            await client.chat("Hello", model="test-model")
        request = httpx_mock.get_request()
        body = json.loads(request.content)
        assert body["message"] == "Hello"
        assert body["model"] == "test-model"


class TestAsyncContinueChat:
    async def test_continue_chat(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "dag_id": "dag-123",
                "node_id": "node-789",
                "content": "Continued!",
            }
        )
        async with AsyncLangDAGClient() as client:
            resp = await client.continue_chat("dag-123", "Follow up")
            assert isinstance(resp, ChatResponse)
            assert resp.content == "Continued!"


class TestAsyncDeleteDAG:
    async def test_delete_dag(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "deleted", "id": "dag-1"})
        async with AsyncLangDAGClient() as client:
            resp = await client.delete_dag("dag-1")
            assert resp["status"] == "deleted"


class TestAsyncAPIKeyHeader:
    async def test_api_key_sent(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "ok"})
        async with AsyncLangDAGClient(api_key="my-key") as client:
            await client.health()
        request = httpx_mock.get_request()
        assert request.headers["X-API-Key"] == "my-key"
