"""
Backend reader for LangGraph PostgresSaver.

Requires the optional 'postgres' extras:
    pip install langgraph-export[postgres]
"""
from __future__ import annotations

from contextlib import contextmanager
from typing import Iterator


class PostgresBackend:
    """
    Reads thread data from a LangGraph AsyncPostgresSaver / PostgresSaver.

    Connection string format (same as psycopg):
        "postgresql://user:password@host:port/dbname"
    or
        "host=... dbname=... user=... password=..."

    Only the synchronous PostgresSaver is used here to keep the API simple.
    """

    def __init__(self, connection_string: str) -> None:
        self._conn_str = connection_string

    def _psycopg(self):
        try:
            import psycopg  # noqa: F401
        except ImportError as exc:
            raise ImportError(
                "PostgreSQL support requires 'psycopg[binary]>=3.0.0'. "
                "Install with: pip install langgraph-export[postgres]"
            ) from exc
        return psycopg

    @contextmanager
    def _open_saver(self):
        try:
            from langgraph.checkpoint.postgres import PostgresSaver
        except ImportError as exc:
            raise ImportError(
                "PostgreSQL support requires 'langgraph-checkpoint-postgres>=2.0.0'. "
                "Install with: pip install langgraph-export[postgres]"
            ) from exc

        psycopg = self._psycopg()
        with psycopg.connect(self._conn_str) as conn:
            saver = PostgresSaver(conn)
            yield saver

    def thread_ids(self) -> list[str]:
        """Return all distinct thread IDs from the checkpoints table."""
        psycopg = self._psycopg()
        with psycopg.connect(self._conn_str) as conn:
            rows = conn.execute(
                "SELECT DISTINCT thread_id FROM checkpoints ORDER BY thread_id"
            ).fetchall()
            return [r[0] for r in rows]

    def iter_latest_checkpoints(self) -> Iterator[tuple[str, object]]:
        """Yield (thread_id, checkpoint_tuple) for each thread."""
        thread_ids = self.thread_ids()
        if not thread_ids:
            return

        with self._open_saver() as saver:
            for tid in thread_ids:
                config = {"configurable": {"thread_id": tid}}
                results = list(saver.list(config, limit=1))
                cp = results[0] if results else saver.get_tuple(config)
                if cp is not None:
                    yield tid, cp
