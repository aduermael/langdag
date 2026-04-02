"""Cross-SDK graceful degradation tests — verify the Python SDK makes
accumulated content available when streams terminate abnormally.
Identical scenarios tested in Go and TypeScript SDKs.
"""

from langdag.client import _parse_sse_stream
from langdag.models import SSEEventType

# Fixture: stream ends without done event
FIXTURE_NO_DONE = (
    "event: start\ndata: {}\n\n"
    'event: delta\ndata: {"content":"Hello "}\n\n'
    'event: delta\ndata: {"content":"world!"}\n\n'
)

# Fixture: stream ends with error event
FIXTURE_ERROR_TERMINATION = (
    "event: start\ndata: {}\n\n"
    'event: delta\ndata: {"content":"before "}\n\n'
    'event: delta\ndata: {"content":"error"}\n\n'
    "event: error\ndata: connection reset by peer\n\n"
)

# Fixture: empty response (start only)
FIXTURE_EMPTY_RESPONSE = "event: start\ndata: {}\n\n"


def parse_fixture(fixture: str) -> list:
    return list(_parse_sse_stream(iter(fixture.splitlines())))


class TestGracefulDegNoDoneEvent:
    def test_content_available(self):
        events = parse_fixture(FIXTURE_NO_DONE)
        content = "".join(
            e.content for e in events if e.event == SSEEventType.DELTA
        )
        assert content == "Hello world!"

    def test_iteration_completes(self):
        """Parser must not hang when no done event is received."""
        events = parse_fixture(FIXTURE_NO_DONE)
        assert len(events) == 3  # start + 2 deltas

    def test_no_node_id(self):
        events = parse_fixture(FIXTURE_NO_DONE)
        for e in events:
            assert e.node_id is None


class TestGracefulDegErrorTermination:
    def test_content_preserved(self):
        events = parse_fixture(FIXTURE_ERROR_TERMINATION)
        content = "".join(
            e.content for e in events if e.event == SSEEventType.DELTA
        )
        assert content == "before error"

    def test_error_message_surfaced(self):
        events = parse_fixture(FIXTURE_ERROR_TERMINATION)
        error_events = [e for e in events if e.event == SSEEventType.ERROR]
        assert len(error_events) == 1
        assert error_events[0].data["message"] == "connection reset by peer"

    def test_event_count(self):
        events = parse_fixture(FIXTURE_ERROR_TERMINATION)
        assert len(events) == 4  # start + 2 deltas + error


class TestGracefulDegEmptyResponse:
    def test_no_content(self):
        events = parse_fixture(FIXTURE_EMPTY_RESPONSE)
        deltas = [e for e in events if e.event == SSEEventType.DELTA]
        assert len(deltas) == 0

    def test_iteration_completes(self):
        events = parse_fixture(FIXTURE_EMPTY_RESPONSE)
        assert len(events) == 1  # just start
