"""Cross-SDK consistency tests — verify the Python SDK parses SSE fixtures
identically to the Go and TypeScript SDKs. The exact same SSE byte strings
are used in sdks/go/cross_sdk_test.go and sdks/typescript/src/cross-sdk.test.ts.
"""

from langdag.client import _parse_sse_stream
from langdag.models import SSEEventType

# Canonical SSE fixtures — byte-for-byte identical across all 3 SDKs.
# Python's _parse_sse_stream takes an Iterator[str] of lines, so we split.

FIXTURE_NORMAL = (
    "event: start\ndata: {}\n\n"
    'event: delta\ndata: {"content":"Hello "}\n\n'
    'event: delta\ndata: {"content":"world!"}\n\n'
    'event: done\ndata: {"node_id":"test-node-1"}\n\n'
)

FIXTURE_ERROR_MID_STREAM = (
    "event: start\ndata: {}\n\n"
    'event: delta\ndata: {"content":"partial "}\n\n'
    "event: error\ndata: provider crashed\n\n"
)

FIXTURE_MULTI_LINE_ERROR = (
    "event: start\ndata: {}\n\n"
    "event: error\ndata: line one\ndata: line two\ndata: line three\n\n"
)

FIXTURE_ERROR_ONLY = "event: error\ndata: unauthorized\n\n"


def parse_fixture(fixture: str) -> list:
    """Parse an SSE fixture string into events."""
    lines = iter(fixture.splitlines())
    return list(_parse_sse_stream(lines))


class TestCrossSDKNormalFlow:
    def test_event_count_and_types(self):
        events = parse_fixture(FIXTURE_NORMAL)
        assert len(events) == 4
        assert events[0].event == SSEEventType.START
        assert events[1].event == SSEEventType.DELTA
        assert events[2].event == SSEEventType.DELTA
        assert events[3].event == SSEEventType.DONE

    def test_delta_content(self):
        events = parse_fixture(FIXTURE_NORMAL)
        assert events[1].content == "Hello "
        assert events[2].content == "world!"

    def test_accumulated_content(self):
        events = parse_fixture(FIXTURE_NORMAL)
        content = "".join(e.content for e in events if e.event == SSEEventType.DELTA)
        assert content == "Hello world!"

    def test_node_id(self):
        events = parse_fixture(FIXTURE_NORMAL)
        assert events[3].node_id == "test-node-1"

    def test_no_error_events(self):
        events = parse_fixture(FIXTURE_NORMAL)
        error_events = [e for e in events if e.event == SSEEventType.ERROR]
        assert len(error_events) == 0


class TestCrossSDKErrorMidStream:
    def test_event_count_and_types(self):
        events = parse_fixture(FIXTURE_ERROR_MID_STREAM)
        assert len(events) == 3
        assert events[0].event == SSEEventType.START
        assert events[1].event == SSEEventType.DELTA
        assert events[2].event == SSEEventType.ERROR

    def test_partial_content_preserved(self):
        events = parse_fixture(FIXTURE_ERROR_MID_STREAM)
        assert events[1].content == "partial "

    def test_error_message(self):
        events = parse_fixture(FIXTURE_ERROR_MID_STREAM)
        # Python wraps non-JSON error data in {"message": ...}
        assert events[2].data["message"] == "provider crashed"

    def test_no_node_id(self):
        events = parse_fixture(FIXTURE_ERROR_MID_STREAM)
        done_events = [e for e in events if e.event == SSEEventType.DONE]
        assert len(done_events) == 0


class TestCrossSDKMultiLineError:
    def test_event_count_and_types(self):
        events = parse_fixture(FIXTURE_MULTI_LINE_ERROR)
        assert len(events) == 2
        assert events[0].event == SSEEventType.START
        assert events[1].event == SSEEventType.ERROR

    def test_multi_line_error_joined(self):
        events = parse_fixture(FIXTURE_MULTI_LINE_ERROR)
        # Multiple data: lines are joined with \n
        assert events[1].data["message"] == "line one\nline two\nline three"

    def test_no_content(self):
        events = parse_fixture(FIXTURE_MULTI_LINE_ERROR)
        delta_events = [e for e in events if e.event == SSEEventType.DELTA]
        assert len(delta_events) == 0


class TestCrossSDKErrorOnly:
    def test_single_error_event(self):
        events = parse_fixture(FIXTURE_ERROR_ONLY)
        assert len(events) == 1
        assert events[0].event == SSEEventType.ERROR

    def test_error_message(self):
        events = parse_fixture(FIXTURE_ERROR_ONLY)
        assert events[0].data["message"] == "unauthorized"

    def test_no_content(self):
        events = parse_fixture(FIXTURE_ERROR_ONLY)
        delta_events = [e for e in events if e.event == SSEEventType.DELTA]
        assert len(delta_events) == 0
