# LangDAG Python SDK Example

This example demonstrates the LangDAG Python SDK, showing how to:

- Start a conversation with `client.prompt()`
- Continue from a node with `node.prompt()`
- Stream responses with `node.prompt_stream()`
- Branch from an earlier node to explore alternative paths
- List root nodes and examine tree structure

## Prerequisites

1. **Start the LangDAG server:**
   ```bash
   langdag serve
   ```
   The server runs at `http://localhost:8080` by default.

2. **Install dependencies:**
   ```bash
   cd examples/python
   pip install -r requirements.txt
   ```

## Running the Example

```bash
python example.py
```

## What the Example Does

1. **Starts a conversation** asking about Python vs Rust differences
2. **Continues the conversation** asking about web server recommendations
3. **Branches from the first response** to ask about data science instead
4. **Lists root nodes** to show conversation history
5. **Displays the tree structure** showing the branching paths

## SDK Features Demonstrated

| Feature | Method | Description |
|---------|--------|-------------|
| Prompt | `client.prompt()` | Start a new conversation |
| Continue | `node.prompt()` | Continue from any node |
| Stream | `node.prompt_stream()` | Stream from any node |
| List Roots | `client.list_roots()` | Get all conversations |
| Get Tree | `client.get_tree()` | Get full tree with nodes |
