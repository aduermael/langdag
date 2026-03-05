"""
Backend reader for LangGraph InMemorySaver.
"""
from __future__ import annotations

from typing import Iterator

from langgraph.checkpoint.memory import InMemorySaver


class MemoryBackend:
    """
    Reads thread data from a LangGraph InMemorySaver instance.

    The InMemorySaver keeps checkpoints in memory as:
        storage[thread_id][checkpoint_ns][checkpoint_id] = CheckpointTuple

    We use the public saver.list() API to enumerate threads and retrieve the
    latest checkpoint for each thread.
    """

    def __init__(self, saver: InMemorySaver) -> None:
        self._saver = saver

    def thread_ids(self) -> list[str]:
        """Return all distinct thread IDs known to the saver."""
        seen: set[str] = set()
        # saver.storage is a defaultdict:
        #   thread_id -> namespace -> checkpoint_id -> CheckpointTuple
        for thread_id in self._saver.storage:
            seen.add(thread_id)
        return list(seen)

    def latest_checkpoint(self, thread_id: str):
        """
        Return the latest CheckpointTuple for a thread, or None.

        We use the saver's list() method which yields CheckpointTuple objects
        in reverse-chronological order; the first result is the most recent.
        """
        config = {"configurable": {"thread_id": thread_id}}
        results = list(self._saver.list(config, limit=1))
        if not results:
            # Fallback: try get_tuple
            return self._saver.get_tuple(config)
        return results[0]

    def iter_latest_checkpoints(self) -> Iterator[tuple[str, object]]:
        """Yield (thread_id, checkpoint_tuple) for each thread."""
        for tid in self.thread_ids():
            cp = self.latest_checkpoint(tid)
            if cp is not None:
                yield tid, cp
