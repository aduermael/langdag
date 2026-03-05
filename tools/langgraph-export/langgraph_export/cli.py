"""
Command-line interface for langgraph-export.

Usage::

    # Export from SQLite
    langgraph-export --sqlite path/to/langgraph.db --output export.json

    # Export from PostgreSQL
    langgraph-export --postgres "postgresql://user:pass@host/db" --output export.json

    # Print to stdout (omit --output)
    langgraph-export --sqlite path/to/langgraph.db
"""
from __future__ import annotations

import argparse
import sys

from langgraph_export.exporter import LangGraphExporter


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="langgraph-export",
        description="Export LangGraph conversation data to langdag JSON format.",
    )
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument(
        "--sqlite",
        metavar="DB_PATH",
        help="Path to a LangGraph SQLite checkpoint database.",
    )
    source.add_argument(
        "--postgres",
        metavar="CONNECTION_STRING",
        help="PostgreSQL connection string for a LangGraph checkpoint database.",
    )
    parser.add_argument(
        "--output",
        "-o",
        metavar="FILE",
        default=None,
        help="Output JSON file path. Defaults to stdout.",
    )
    parser.add_argument(
        "--indent",
        type=int,
        default=2,
        metavar="N",
        help="JSON indentation level (default: 2). Use 0 for compact output.",
    )
    return parser


def main(argv: list[str] | None = None) -> None:
    parser = _build_parser()
    args = parser.parse_args(argv)

    if args.sqlite:
        exporter = LangGraphExporter.from_sqlite(args.sqlite)
    else:
        exporter = LangGraphExporter.from_postgres(args.postgres)

    export = exporter.export()
    indent: int | None = args.indent if args.indent > 0 else None

    import json
    text = json.dumps(export.to_dict(), indent=indent)

    if args.output:
        with open(args.output, "w", encoding="utf-8") as fh:
            fh.write(text)
        print(f"Exported {len(export.threads)} thread(s) to {args.output}", file=sys.stderr)
    else:
        print(text)


if __name__ == "__main__":
    main()
