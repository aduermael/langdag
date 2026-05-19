"""Unit tests for the synchronous LangDAG client."""

import json

import httpx
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
                "output_group_id": "22222222-2222-2222-2222-222222222222",
            }
        )
        client = LangDAGClient()
        node = client.get_node("node-1")
        assert node.id == "node-1"
        assert node.content == "Hello"
        assert node.title == "My conversation"
        assert node.system_prompt == "Be helpful"
        assert node.output_group_id == "22222222-2222-2222-2222-222222222222"


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
                "output_group_id": "11111111-1111-1111-1111-111111111111",
            }
        )
        client = LangDAGClient()
        resp = client.prompt("Hello")
        assert isinstance(resp, PromptResponse)
        assert resp.node_id == "node-456"
        assert resp.content == "Hello back!"
        assert resp.tokens_in == 5
        assert resp.tokens_out == 3
        assert resp.output_group_id == "11111111-1111-1111-1111-111111111111"

    def test_prompt_sends_correct_body(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-1",
                "content": "ok",
            }
        )
        client = LangDAGClient()
        client.prompt(
            "Hello",
            model="test-model",
            system_prompt="Be nice",
            tools=[{"name": "web_search"}],
        )
        request = httpx_mock.get_request()
        body = json.loads(request.content)
        assert body["message"] == "Hello"
        assert body["model"] == "test-model"
        assert body["system_prompt"] == "Be nice"
        assert body["stream"] is False
        assert body["tools"] == [{"name": "web_search"}]


class TestPromptFrom:
    def test_prompt_from(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-789",
                "content": "Continued!",
                "tokens_in": 10,
                "tokens_out": 5,
                "output_group_id": "33333333-3333-3333-3333-333333333333",
            }
        )
        client = LangDAGClient()
        resp = client.prompt_from("node-123", "Follow up")
        assert isinstance(resp, PromptResponse)
        assert resp.node_id == "node-789"
        assert resp.content == "Continued!"
        assert resp.output_group_id == "33333333-3333-3333-3333-333333333333"

    def test_prompt_from_sends_correct_body(self, httpx_mock: HTTPXMock):
        httpx_mock.add_response(
            json={
                "node_id": "node-1",
                "content": "ok",
            }
        )
        client = LangDAGClient()
        client.prompt_from(
            "node-123",
            "Follow up",
            model="test-model",
            tools=[{"name": "web_search"}],
        )
        request = httpx_mock.get_request()
        body = json.loads(request.content)
        assert body["message"] == "Follow up"
        assert body["model"] == "test-model"
        assert body["stream"] is False
        assert body["tools"] == [{"name": "web_search"}]
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


# --- Phase 10: Python SDK Error Handling & SSE Edge Cases ---


class TestStreamWithoutDoneEvent:
    """10a: SSE stream without done event — verify iteration completes,
    content from deltas is accessible, no node_id available."""

    def test_sync_stream_no_done_completes(self, httpx_mock: HTTPXMock):
        """Stream with start + deltas but no done event should complete
        iteration without hanging."""
        sse_body = (
            "event: start\ndata: {}\n\n"
            'event: delta\ndata: {"content":"Hello "}\n\n'
            'event: delta\ndata: {"content":"world!"}\n\n'
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        client = LangDAGClient()
        events = list(client.prompt("Hello", stream=True))
        assert len(events) == 3  # start + 2 deltas, no done
        assert events[0].event == SSEEventType.START
        assert events[1].content == "Hello "
        assert events[2].content == "world!"
        # No done event means no node_id from any event
        assert all(e.node_id is None for e in events)

    def test_sync_stream_no_done_content_accessible(self, httpx_mock: HTTPXMock):
        """Accumulated content from deltas is fully accessible even without
        a done event."""
        sse_body = (
            "event: start\ndata: {}\n\n"
            'event: delta\ndata: {"content":"partial "}\n\n'
            'event: delta\ndata: {"content":"content"}\n\n'
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        client = LangDAGClient()
        content = ""
        for event in client.prompt("Hello", stream=True):
            if event.content:
                content += event.content
        assert content == "partial content"

    def test_parser_no_done_event(self):
        """Parser-level: stream ends without done, iteration completes."""
        lines = iter([
            "event: start",
            "data: {}",
            "",
            "event: delta",
            'data: {"content":"abc"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 2
        assert events[1].content == "abc"


class TestProviderErrorMidStream:
    """10b: Provider error mid-stream — error event yielded with message,
    prior delta content available."""

    def test_sync_error_after_deltas(self, httpx_mock: HTTPXMock):
        """Server sends start + 2 deltas + error event. All events yielded,
        prior content accessible."""
        sse_body = (
            "event: start\ndata: {}\n\n"
            'event: delta\ndata: {"content":"Hello "}\n\n'
            'event: delta\ndata: {"content":"world"}\n\n'
            'event: error\ndata: {"message":"provider crashed"}\n\n'
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        client = LangDAGClient()
        events = list(client.prompt("Hello", stream=True))
        assert len(events) == 4
        # Prior deltas preserved
        content = "".join(e.content for e in events if e.content)
        assert content == "Hello world"
        # Error event has the message
        assert events[3].event == SSEEventType.ERROR
        assert events[3].data["message"] == "provider crashed"

    def test_sync_error_plain_text_data(self, httpx_mock: HTTPXMock):
        """Error event with plain text (not JSON) data should be wrapped
        in {"message": ...}."""
        sse_body = (
            "event: start\ndata: {}\n\n"
            'event: delta\ndata: {"content":"partial"}\n\n'
            "event: error\ndata: something went wrong\n\n"
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        client = LangDAGClient()
        events = list(client.prompt("Hello", stream=True))
        assert events[2].event == SSEEventType.ERROR
        assert events[2].data == {"message": "something went wrong"}

    def test_parser_error_mid_stream(self):
        """Parser-level: error event after deltas."""
        lines = iter([
            "event: start",
            "data: {}",
            "",
            "event: delta",
            'data: {"content":"before"}',
            "",
            "event: error",
            'data: {"message":"provider crashed"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 3
        assert events[1].content == "before"
        assert events[2].event == SSEEventType.ERROR
        assert events[2].data["message"] == "provider crashed"


class TestConnectionTimeout:
    """10c: Connection timeout during stream — raises ConnectionError or
    httpx timeout, not an unhandled exception."""

    def test_sync_connect_timeout(self):
        """Client with very short timeout connecting to unreachable host
        should raise ConnectionError."""
        client = LangDAGClient(base_url="http://localhost:1", timeout=0.001)
        with pytest.raises(ConnectionError):
            list(client.prompt("Hello", stream=True))

    def test_sync_read_timeout(self, httpx_mock: HTTPXMock):
        """Read timeout during streaming should raise an exception, not hang.
        pytest-httpx raises httpx.TimeoutException when no response configured
        after a real timeout, but we can test that the exception type is sensible
        by configuring a callback that times out."""
        # httpx.TimeoutException is a base for read timeouts
        httpx_mock.add_exception(httpx.ReadTimeout("read timed out"))
        client = LangDAGClient()
        with pytest.raises(httpx.ReadTimeout):
            list(client.prompt("Hello", stream=True))


class TestInvalidSSESequence:
    """10d: Invalid SSE event sequences — delta before start, done without
    deltas, multiple done events. Verify graceful handling."""

    def test_delta_before_start(self):
        """Delta events before start should still be yielded — parser
        doesn't enforce ordering."""
        lines = iter([
            "event: delta",
            'data: {"content":"early"}',
            "",
            "event: start",
            "data: {}",
            "",
            "event: done",
            'data: {"node_id":"n-1"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 3
        assert events[0].event == SSEEventType.DELTA
        assert events[0].content == "early"
        assert events[1].event == SSEEventType.START
        assert events[2].event == SSEEventType.DONE

    def test_done_without_deltas(self):
        """Done event immediately after start (no deltas) — valid sequence."""
        lines = iter([
            "event: start",
            "data: {}",
            "",
            "event: done",
            'data: {"node_id":"n-empty"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 2
        assert events[1].event == SSEEventType.DONE
        assert events[1].node_id == "n-empty"

    def test_multiple_done_events(self):
        """Multiple done events — all yielded, parser doesn't stop at first."""
        lines = iter([
            "event: start",
            "data: {}",
            "",
            "event: done",
            'data: {"node_id":"n-1"}',
            "",
            "event: done",
            'data: {"node_id":"n-2"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        assert len(events) == 3
        assert events[1].node_id == "n-1"
        assert events[2].node_id == "n-2"

    def test_empty_data_lines_skipped(self):
        """Event with event type but no data lines should not yield."""
        lines = iter([
            "event: delta",
            "",
            "event: done",
            'data: {"node_id":"n-1"}',
            "",
        ])
        events = list(_parse_sse_stream(lines))
        # First event has no data lines, so it's skipped
        assert len(events) == 1
        assert events[0].event == SSEEventType.DONE

    def test_data_without_event_type_skipped(self):
        """Data lines without a preceding event type should not yield."""
        lines = iter([
            "data: {}",
            "",
            "event: start",
            "data: {}",
            "",
        ])
        events = list(_parse_sse_stream(lines))
        # First block has no event type, so it's skipped
        assert len(events) == 1
        assert events[0].event == SSEEventType.START

    def test_sync_stream_delta_before_start(self, httpx_mock: HTTPXMock):
        """Full client: delta before start is yielded without crash."""
        sse_body = (
            'event: delta\ndata: {"content":"early"}\n\n'
            "event: start\ndata: {}\n\n"
            'event: done\ndata: {"node_id":"n-1"}\n\n'
        )
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        client = LangDAGClient()
        events = list(client.prompt("Hello", stream=True))
        assert len(events) == 3
        assert events[0].event == SSEEventType.DELTA
        assert events[0].content == "early"


class TestLargeStreamedResponse:
    """10e: Large streamed response — 10,000 delta events, all content
    collected correctly, iteration completes."""

    def test_sync_large_stream(self, httpx_mock: HTTPXMock):
        """10,000 delta events should all be yielded with correct content."""
        delta_count = 10_000
        parts = ["event: start\ndata: {}\n\n"]
        for i in range(delta_count):
            parts.append(f'event: delta\ndata: {{"content":"chunk{i} "}}\n\n')
        parts.append('event: done\ndata: {"node_id":"n-big"}\n\n')
        sse_body = "".join(parts)
        httpx_mock.add_response(
            status_code=200,
            content=sse_body.encode(),
            headers={"content-type": "text/event-stream"},
        )
        client = LangDAGClient()
        content = ""
        node_id = None
        event_count = 0
        for event in client.prompt("Hello", stream=True):
            event_count += 1
            if event.content:
                content += event.content
            if event.node_id:
                node_id = event.node_id
        # start + 10000 deltas + done = 10002
        assert event_count == delta_count + 2
        assert node_id == "n-big"
        # Verify first and last chunks present
        assert content.startswith("chunk0 ")
        assert f"chunk{delta_count - 1} " in content

    def test_parser_large_stream(self):
        """Parser-level: 10,000 deltas with lazy iteration (generator)."""
        delta_count = 10_000

        def gen_lines():
            yield "event: start"
            yield "data: {}"
            yield ""
            for i in range(delta_count):
                yield "event: delta"
                yield f'{{"content":"c{i}"}}'[0:0] + f'data: {{"content":"c{i}"}}'
                yield ""
            yield "event: done"
            yield 'data: {"node_id":"n-1"}'
            yield ""

        content_parts = []
        for event in _parse_sse_stream(gen_lines()):
            if event.content:
                content_parts.append(event.content)
        assert len(content_parts) == delta_count
        assert content_parts[0] == "c0"
        assert content_parts[-1] == f"c{delta_count - 1}"
