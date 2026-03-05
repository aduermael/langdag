"""
Backend reader for LangGraph SqliteSaver.
"""
from __future__ import annotations

import sqlite3
from contextlib import contextmanager
from typing import Iterator


class SqliteBackend:
    """
    Reads thread data from a LangGraph SqliteSaver SQLite database.

    Rather than instantiating SqliteSaver (which manages its own connection
    lifecycle), we open a raw sqlite3 connection to query thread IDs directly,
    then use SqliteSaver to retrieve the latest checkpoint per thread.
    """

    def __init__(self, db_path: str) -> None:
        self._db_path = db_path
        self._saver = None  # opened lazily via context manager

    @contextmanager
    def _open_saver(self):
        from langgraph.checkpoint.sqlite import SqliteSaver

        with SqliteSaver.from_conn_string(self._db_path) as saver:
            yield saver

    def thread_ids(self) -> list[str]:
        """Return all distinct thread IDs from the checkpoints table."""
        conn = sqlite3.connect(self._db_path)
        try:
            rows = conn.execute(
                "SELECT DISTINCT thread_id FROM checkpoints ORDER BY thread_id"
            ).fetchall()
            return [r[0] for r in rows]
        except sqlite3.OperationalError:
            # Table may not exist yet (empty DB)
            return []
        finally:
            conn.close()

    def iter_latest_checkpoints(self) -> Iterator[tuple[str, object]]:
        """
        Yield (thread_id, checkpoint_tuple) for each thread using a single
        SqliteSaver context so the connection stays open across all reads.
        """
        thread_ids = self.thread_ids()
        if not thread_ids:
            return

        with self._open_saver() as saver:
            for tid in thread_ids:
                config = {"configurable": {"thread_id": tid}}
                # list() returns newest first; limit=1 gives us the latest
                results = list(saver.list(config, limit=1))
                cp = results[0] if results else saver.get_tuple(config)
                if cp is not None:
                    yield tid, cp
