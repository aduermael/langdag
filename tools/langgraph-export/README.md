# langgraph-export

Export LangGraph conversation data to the langdag JSON format so it can be
imported into a langdag database.

## Installation

```bash
pip install .
# With PostgreSQL support
pip install ".[postgres]"
```

## Requirements

- Python >= 3.11
- `langgraph >= 0.2.0`
- `langgraph-checkpoint-sqlite >= 2.0.0`

## Quick start

### Python API

```python
from langgraph_export import LangGraphExporter

# From a SQLite checkpoint database
exporter = LangGraphExporter.from_sqlite("path/to/langgraph.db")
export = exporter.export()
export.save("export.json")

# From an InMemorySaver (useful in tests / scripts)
from langgraph.checkpoint.memory import InMemorySaver
saver = InMemorySaver()
# ... run your graphs ...
exporter = LangGraphExporter.from_memory(saver)
export = exporter.export()
export.save("export.json")

# From PostgreSQL (requires extras)
exporter = LangGraphExporter.from_postgres("postgresql://user:pass@host/db")
export = exporter.export()
export.save("export.json")
```

### CLI

```bash
# Export SQLite database to a file
langgraph-export --sqlite path/to/langgraph.db --output export.json

# Export to stdout (pipe-friendly)
langgraph-export --sqlite path/to/langgraph.db | jq '.threads | length'

# Export PostgreSQL database
langgraph-export --postgres "postgresql://user:pass@host/db" --output export.json

# Compact JSON output
langgraph-export --sqlite langgraph.db --indent 0 --output export.json
```

### Import into langdag

After exporting, use the langdag CLI to import:

```bash
langdag import langgraph --file export.json --output langdag.db
```

## Export format

```json
{
  "version": "1",
  "source_type": "langgraph",
  "exported_at": "2026-03-04T12:00:00Z",
  "threads": [
    {
      "thread_id": "abc-123",
      "created_at": "2026-03-01T10:00:00Z",
      "messages": [
        {
          "id": "msg-1",
          "role": "user",
          "content": "Hello!"
        },
        {
          "id": "msg-2",
          "role": "assistant",
          "content": "Hi there!",
          "model": "claude-3-5-sonnet-20241022",
          "tokens_in": 10,
          "tokens_out": 5
        }
      ],
      "metadata": {
        "original_thread_id": "abc-123"
      }
    }
  ]
}
```

### Role mapping

| LangGraph type  | Export role  |
|-----------------|--------------|
| `HumanMessage`  | `user`       |
| `AIMessage`     | `assistant`  |
| `ToolMessage`   | `tool`       |
| `SystemMessage` | `system`     |

### Tool calls

```json
{
  "id": "msg-ai",
  "role": "assistant",
  "content": "",
  "tool_calls": [
    {
      "id": "call_abc",
      "name": "search",
      "input": {"query": "python tutorials"}
    }
  ]
}
```

```json
{
  "id": "msg-tool",
  "role": "tool",
  "content": "Results...",
  "tool_call_id": "call_abc",
  "tool_name": "search"
}
```

## Running tests

```bash
pip install -e .
pytest
```

Tests use `InMemorySaver` and a temporary SQLite database; no external services
are required.
