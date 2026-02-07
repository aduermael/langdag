"""Unit tests for the synchronous LangDAG client."""

import json

import pytest
from pytest_httpx import HTTPXMock

from langdag.client import LangDAGClient, _parse_sse_stream
from langdag.exceptions import (
    APIError,
    AuthenticationError,
    BadRequestError,
    ConnectionError,
    NotFoundError,
)
from langdag.models import ChatResponse, SSEEventType


class TestClientInit:
    def test_default_base_url(self):
        client = LangDAGClient()
        assert client.base_url == "http://localhost:8080"

    def test_custom_base_url(self):
        client = LangDAGClient(base_url="http://example.com:3000")
        assert client.base_url == "http://example.com:3000"

    def test_trailing_slash_stripped(self):
        client = LangDAGClient(base_url="http://localhost:8080/")
        assert client.base_url == "http://localhost:8080"

    def test_api_key_stored(self):
        client = LangDAGClient(api_key="test-key")
        assert client.api_key == "test-key"

    def test_context_manager(self):
        with LangDAGClient() as client:
            assert client._client is None
        assert client._client is None


class TestHealth:
    def test_health_ok(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "ok"})
        client = LangDAGClient()
        result = client.health()
        assert result["status"] == "ok"


class TestErrorHandling:
    def test_authentication_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=401,
            json={"error": "unauthorized"},
        )
        client = LangDAGClient()
        with pytest.raises(AuthenticationError) as exc_info:
            client.health()
        assert exc_info.value.status_code == 401

    def test_not_found_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=404,
            json={"error": "not found"},
        )
        client = LangDAGClient()
        with pytest.raises(NotFoundError) as exc_info:
            client.get_dag("nonexistent")
        assert exc_info.value.status_code == 404

    def test_bad_request_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=400,
            json={"error": "invalid request"},
        )
        client = LangDAGClient()
        with pytest.raises(BadRequestError) as exc_info:
            client.chat("test")
        assert exc_info.value.status_code == 400

    def test_generic_api_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=500,
            json={"error": "internal server error"},
        )
        client = LangDAGClient()
        with pytest.raises(APIError) as exc_info:
            client.health()
        assert exc_info.value.status_code == 500

    def test_connection_error(self):
        client = LangDAGClient(base_url="http://localhost:1")
        with pytest.raises(ConnectionError):
            client.health()


class TestListDAGs:
    def test_list_dags(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json=[
                {
                    "id": "dag-1",
                    "status": "completed",
                    "created_at": "2024-01-01T00:00:00Z",
                    "updated_at": "2024-01-01T00:00:00Z",
                },
                {
                    "id": "dag-2",
                    "status": "running",
                    "created_at": "2024-01-02T00:00:00Z",
                    "updated_at": "2024-01-02T00:00:00Z",
                },
            ]
        )
        client = LangDAGClient()
        dags = client.list_dags()
        assert len(dags) == 2
        assert dags[0].id == "dag-1"
        assert dags[1].id == "dag-2"


class TestGetDAG:
    def test_get_dag(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "id": "dag-1",
                "status": "completed",
                "created_at": "2024-01-01T00:00:00Z",
                "updated_at": "2024-01-01T00:00:00Z",
                "nodes": [
                    {
                        "id": "node-1",
                        "sequence": 0,
                        "node_type": "user",
                        "content": "Hello",
                        "created_at": "2024-01-01T00:00:00Z",
                    }
                ],
            }
        )
        client = LangDAGClient()
        dag = client.get_dag("dag-1")
        assert dag.id == "dag-1"
        assert len(dag.nodes) == 1
        assert dag.nodes[0].content == "Hello"


class TestChat:
    def test_chat_non_streaming(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "dag_id": "dag-123",
                "node_id": "node-456",
                "content": "Hello back!",
                "tokens_in": 5,
                "tokens_out": 3,
            }
        )
        client = LangDAGClient()
        resp = client.chat("Hello")
        assert isinstance(resp, ChatResponse)
        assert resp.dag_id == "dag-123"
        assert resp.content == "Hello back!"

    def test_chat_sends_correct_body(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "dag_id": "dag-1",
                "node_id": "node-1",
                "content": "ok",
            }
        )
        client = LangDAGClient()
        client.chat("Hello", model="test-model", system_prompt="Be nice")
        request = httpx_mock.get_request()
        body = json.loads(request.content)
        assert body["message"] == "Hello"
        assert body["model"] == "test-model"
        assert body["system_prompt"] == "Be nice"
        assert body["stream"] is False


class TestContinueChat:
    def test_continue_chat(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "dag_id": "dag-123",
                "node_id": "node-789",
                "content": "Continued!",
            }
        )
        client = LangDAGClient()
        resp = client.continue_chat("dag-123", "Follow up")
        assert isinstance(resp, ChatResponse)
        assert resp.content == "Continued!"


class TestDeleteDAG:
    def test_delete_dag(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "deleted", "id": "dag-1"})
        client = LangDAGClient()
        resp = client.delete_dag("dag-1")
        assert resp["status"] == "deleted"


class TestAPIKeyHeader:
    def test_api_key_sent(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "ok"})
        client = LangDAGClient(api_key="my-secret-key")
        client.health()
        request = httpx_mock.get_request()
        assert request.headers["X-API-Key"] == "my-secret-key"


class TestSSEParsing:
    def test_parse_start_delta_done(self):
        lines = iter([
            "event: start",
            'data: {"dag_id":"dag-1"}',
            "",
            "event: delta",
            'data: {"content":"Hello "}',
            "",
            "event: delta",
            'data: {"content":"world!"}',
            "",
            "event: done",
            'data: {"dag_id":"dag-1","node_id":"node-1"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 4
        assert events[0].event == SSEEventType.START
        assert events[0].dag_id == "dag-1"
        assert events[1].event == SSEEventType.DELTA
        assert events[1].content == "Hello "
        assert events[2].content == "world!"
        assert events[3].event == SSEEventType.DONE
        assert events[3].node_id == "node-1"

    def test_parse_error_event(self):
        lines = iter([
            "event: error",
            "data: something went wrong",
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 1
        assert events[0].event == SSEEventType.ERROR

    def test_parse_unknown_event_skipped(self):
        lines = iter([
            "event: unknown_event",
            "data: {}",
            "",
            "event: start",
            'data: {"dag_id":"dag-1"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 1
        assert events[0].event == SSEEventType.START

    def test_parse_empty_stream(self):
        events = list(_parse_sse_stream(iter([])))
        assert len(events) == 0
