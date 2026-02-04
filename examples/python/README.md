# LangDAG Python SDK Example

This example demonstrates the LangDAG Python SDK, showing how to:

- Start a conversation with streaming responses
- Continue an existing conversation
- Fork from an earlier node to explore alternative paths
- List all DAGs and examine conversation structure

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
3. **Forks from the first response** to ask about data science instead
4. **Lists all DAGs** to show conversation history
5. **Displays the DAG structure** showing the branching paths

## Expected Output

The example produces output showing:
- Streaming responses from the LLM
- Node and DAG IDs for tracking
- A visual representation of the conversation branching structure

## SDK Features Demonstrated

| Feature | Method | Description |
|---------|--------|-------------|
| Chat | `client.chat()` | Start a new conversation |
| Continue | `client.continue_chat()` | Continue from the latest node |
| Fork | `client.fork_chat()` | Branch from any node |
| List DAGs | `client.list_dags()` | Get all conversations |
| Get DAG | `client.get_dag()` | Get full DAG with nodes |

## Customization

You can modify `example.py` to:
- Connect to a different server URL
- Use API key authentication: `LangDAGClient(api_key="your-key")`
- Change the conversation topic
- Explore more SDK features like workflows
