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
from langdag.models import PromptResponse, SSEEventType


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
            client.get_node("nonexistent")
        assert exc_info.value.status_code == 404

    def test_bad_request_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=400,
            json={"error": "invalid request"},
        )
        client = LangDAGClient()
        with pytest.raises(BadRequestError) as exc_info:
            client.prompt("test")
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


class TestListRoots:
    def test_list_roots(self, httpx_mock: HTTPXMock):
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
                {
                    "id": "node-2",
                    "sequence": 0,
                    "node_type": "user",
                    "content": "Hi there",
                    "created_at": "2024-01-02T00:00:00Z",
                    "title": "Second conversation",
                },
            ]
        )
        client = LangDAGClient()
        roots = client.list_roots()
        assert len(roots) == 2
        assert roots[0].id == "node-1"
        assert roots[0].title == "First conversation"
        assert roots[1].id == "node-2"


class TestGetNode:
    def test_get_node(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "id": "node-1",
                "sequence": 0,
                "node_type": "user",
                "content": "Hello",
                "created_at": "2024-01-01T00:00:00Z",
                "title": "My conversation",
                "system_prompt": "Be helpful",
            }
        )
        client = LangDAGClient()
        node = client.get_node("node-1")
        assert node.id == "node-1"
        assert node.content == "Hello"
        assert node.title == "My conversation"
        assert node.system_prompt == "Be helpful"


class TestGetTree:
    def test_get_tree(self, httpx_mock: HTTPXMock):
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
        client = LangDAGClient()
        tree = client.get_tree("node-1")
        assert len(tree) == 2
        assert tree[0].id == "node-1"
        assert tree[1].id == "node-2"
        assert tree[1].parent_id == "node-1"


class TestPrompt:
    def test_prompt_non_streaming(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-456",
                "content": "Hello back!",
                "tokens_in": 5,
                "tokens_out": 3,
            }
        )
        client = LangDAGClient()
        resp = client.prompt("Hello")
        assert isinstance(resp, PromptResponse)
        assert resp.node_id == "node-456"
        assert resp.content == "Hello back!"
        assert resp.tokens_in == 5
        assert resp.tokens_out == 3

    def test_prompt_sends_correct_body(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-1",
                "content": "ok",
            }
        )
        client = LangDAGClient()
        client.prompt("Hello", model="test-model", system_prompt="Be nice")
        request = httpx_mock.get_request()
        body = json.loads(request.content)
        assert body["message"] == "Hello"
        assert body["model"] == "test-model"
        assert body["system_prompt"] == "Be nice"
        assert body["stream"] is False


class TestPromptFrom:
    def test_prompt_from(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-789",
                "content": "Continued!",
                "tokens_in": 10,
                "tokens_out": 5,
            }
        )
        client = LangDAGClient()
        resp = client.prompt_from("node-123", "Follow up")
        assert isinstance(resp, PromptResponse)
        assert resp.node_id == "node-789"
        assert resp.content == "Continued!"

    def test_prompt_from_sends_correct_body(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-1",
                "content": "ok",
            }
        )
        client = LangDAGClient()
        client.prompt_from("node-123", "Follow up", model="test-model")
        request = httpx_mock.get_request()
        body = json.loads(request.content)
        assert body["message"] == "Follow up"
        assert body["model"] == "test-model"
        assert body["stream"] is False
        assert request.url.path == "/nodes/node-123/prompt"


class TestDeleteNode:
    def test_delete_node(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(json={"status": "deleted", "id": "node-1"})
        client = LangDAGClient()
        resp = client.delete_node("node-1")
        assert resp["status"] == "deleted"
        assert resp["id"] == "node-1"


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
            "data: {}",
            "",
            "event: delta",
            'data: {"content":"Hello "}',
            "",
            "event: delta",
            'data: {"content":"world!"}',
            "",
            "event: done",
            'data: {"node_id":"node-1"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 4
        assert events[0].event == SSEEventType.START
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
            "data: {}",
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 1
        assert events[0].event == SSEEventType.START

    def test_parse_empty_stream(self):
        events = list(_parse_sse_stream(iter([])))
        assert len(events) == 0


# --- 3a: Streaming unit tests for sync client ---


class TestSyncStreaming:
    def test_prompt_stream_iteration(self, httpx_mock: HTTPXMock):
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
        client = LangDAGClient()
        events = list(client.prompt("Hello", stream=True))
        assert len(events) == 4
        assert events[0].event == SSEEventType.START
        assert events[1].content == "Hello "
        assert events[2].content == "world!"
        assert events[3].event == SSEEventType.DONE
        assert events[3].node_id == "n-1"

    def test_prompt_from_stream_iteration(self, httpx_mock: HTTPXMock):
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
        client = LangDAGClient()
        events = list(client.prompt_from("n-1", "More please", stream=True))
        assert len(events) == 3
        assert events[2].node_id == "n-2"

    def test_stream_error_event(self, httpx_mock: HTTPXMock):
        sse_body = (
            "event: start\ndata: {}\n\n"
            "event: error\ndata: something went wrong\n\n"
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        client = LangDAGClient()
        events = list(client.prompt("Hello", stream=True))
        assert len(events) == 2
        assert events[1].event == SSEEventType.ERROR

    def test_stream_http_error(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=500,
            json={"error": "server error"},
        )
        client = LangDAGClient()
        with pytest.raises(APIError) as exc_info:
            list(client.prompt("Hello", stream=True))
        assert exc_info.value.status_code == 500


# --- 3c: Edge case tests ---


class TestHandleResponseEdgeCases:
    def test_non_json_error_body_401(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=401,
            content=b"Unauthorized",
            headers={"content-type": "text/plain"},
        )
        client = LangDAGClient()
        with pytest.raises(AuthenticationError) as exc_info:
            client.health()
        # Should fall back to default message
        assert "Authentication failed" in str(exc_info.value)

    def test_non_json_error_body_generic(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            status_code=502,
            content=b"<html>Bad Gateway</html>",
            headers={"content-type": "text/html"},
        )
        client = LangDAGClient()
        with pytest.raises(APIError) as exc_info:
            client.health()
        assert exc_info.value.status_code == 502


class TestSSEParsingEdgeCases:
    def test_unknown_event_type_skipped(self):
        lines = iter([
            "event: custom_type",
            "data: {}",
            "",
            "event: delta",
            'data: {"content":"kept"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 1
        assert events[0].content == "kept"

    def test_multiline_data_fields(self):
        lines = iter([
            "event: error",
            "data: line one",
            "data: line two",
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 1
        assert events[0].event == SSEEventType.ERROR
        # Multi-line data gets joined with \n, then treated as non-JSON → {"message": ...}
        assert events[0].data["message"] == "line one\nline two"

    def test_non_json_delta_data(self):
        lines = iter([
            "event: delta",
            "data: not-json-at-all",
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 1
        assert events[0].event == SSEEventType.DELTA
        # Non-JSON data should fall back to {"message": ...}
        assert events[0].data == {"message": "not-json-at-all"}
