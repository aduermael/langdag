#!/usr/bin/env python3
"""LangDAG Python SDK Example.

This example demonstrates the key features of the LangDAG SDK:
1. Starting a conversation with streaming
2. Continuing a conversation
3. Forking from an earlier node to explore alternatives
4. Listing DAGs and exploring the conversation structure

Prerequisites:
- LangDAG server running at http://localhost:8080
- Start the server with: langdag serve
"""

from langdag import LangDAGClient, SSEEventType, NodeType


def print_separator(title: str) -> None:
    """Print a visual separator with a title."""
    print("\n" + "=" * 60)
    print(f"  {title}")
    print("=" * 60 + "\n")


def print_streaming_response(events) -> tuple[str, str, str]:
    """Print streaming response and return dag_id, node_id, and full content."""
    dag_id = ""
    node_id = ""
    content_parts = []

    print("Assistant: ", end="", flush=True)

    for event in events:
        if event.event == SSEEventType.START:
            dag_id = event.dag_id or ""
        elif event.event == SSEEventType.DELTA:
            if event.content:
                print(event.content, end="", flush=True)
                content_parts.append(event.content)
        elif event.event == SSEEventType.DONE:
            node_id = event.node_id or ""
        elif event.event == SSEEventType.ERROR:
            print(f"\n[Error: {event.data}]")

    print("\n")  # End the response line
    return dag_id, node_id, "".join(content_parts)


def print_dag_structure(client: LangDAGClient, dag_id: str) -> None:
    """Print the structure of a DAG showing all nodes."""
    dag = client.get_dag(dag_id)
    print(f"DAG: {dag.id[:8]}...")
    print(f"  Status: {dag.status.value}")
    print(f"  Title: {dag.title or '(untitled)'}")
    print(f"  Nodes: {dag.node_count}")
    print()

    # Build parent-child relationships for visualization
    nodes_by_id = {node.id: node for node in dag.nodes}
    children: dict[str | None, list[str]] = {}

    for node in dag.nodes:
        parent = node.parent_id
        if parent not in children:
            children[parent] = []
        children[parent].append(node.id)

    def print_node(node_id: str, indent: int = 0) -> None:
        """Recursively print a node and its children."""
        node = nodes_by_id[node_id]
        prefix = "  " * indent

        # Truncate content for display
        content_preview = node.content[:50].replace("\n", " ")
        if len(node.content) > 50:
            content_preview += "..."

        node_type_icon = {
            NodeType.USER: "[USER]",
            NodeType.ASSISTANT: "[ASST]",
            NodeType.TOOL_CALL: "[TOOL]",
            NodeType.TOOL_RESULT: "[RSLT]",
        }.get(node.node_type, f"[{node.node_type.value.upper()}]")

        print(f"{prefix}{node_type_icon} {node.id[:8]}... : {content_preview}")

        # Print children
        for child_id in children.get(node_id, []):
            print_node(child_id, indent + 1)

    # Print all root nodes (nodes without parents)
    print("  Node structure:")
    for root_id in children.get(None, []):
        print_node(root_id, indent=2)


def main() -> None:
    """Run the LangDAG SDK demonstration."""
    print("\n" + "#" * 60)
    print("#  LangDAG Python SDK Demo")
    print("#  Demonstrating conversation branching and DAG exploration")
    print("#" * 60)

    # Connect to the LangDAG server
    with LangDAGClient(base_url="http://localhost:8080") as client:

        # Check server health
        try:
            health = client.health()
            print(f"\nServer status: {health.get('status', 'unknown')}")
        except Exception as e:
            print(f"\nError: Could not connect to server: {e}")
            print("Make sure the LangDAG server is running: langdag serve")
            return

        # ============================================================
        # Step 1: Start a new conversation about programming languages
        # ============================================================
        print_separator("Step 1: Starting a new conversation")

        print("User: What are the main differences between Python and Rust?\n")

        events = client.chat(
            message="What are the main differences between Python and Rust? Keep your answer brief.",
            stream=True,
        )

        dag_id, first_response_node, _ = print_streaming_response(events)

        print(f"[Conversation started - DAG ID: {dag_id[:8]}...]")
        print(f"[Response node: {first_response_node[:8]}...]")

        # ============================================================
        # Step 2: Continue the conversation
        # ============================================================
        print_separator("Step 2: Continuing the conversation")

        print("User: Which one would you recommend for building a web server?\n")

        events = client.continue_chat(
            dag_id=dag_id,
            message="Which one would you recommend for building a web server?",
            stream=True,
        )

        _, second_response_node, _ = print_streaming_response(events)

        print(f"[Continued conversation - Node: {second_response_node[:8]}...]")

        # ============================================================
        # Step 3: Fork from the first response to explore an alternative
        # ============================================================
        print_separator("Step 3: Forking to explore an alternative path")

        print(f"[Forking from node {first_response_node[:8]}... to ask a different question]")
        print()
        print("User: Which one would you recommend for data science work?\n")

        events = client.fork_chat(
            dag_id=dag_id,
            node_id=first_response_node,
            message="Which one would you recommend for data science work?",
            stream=True,
        )

        _, fork_response_node, _ = print_streaming_response(events)

        print(f"[Forked conversation - New branch node: {fork_response_node[:8]}...]")

        # ============================================================
        # Step 4: List all DAGs
        # ============================================================
        print_separator("Step 4: Listing all DAGs")

        dags = client.list_dags()
        print(f"Found {len(dags)} DAG(s):\n")

        for dag in dags[:5]:  # Show first 5
            print(f"  - {dag.id[:8]}... | {dag.status.value:10} | {dag.title or '(untitled)'}")

        if len(dags) > 5:
            print(f"  ... and {len(dags) - 5} more")

        # ============================================================
        # Step 5: Show the branching structure
        # ============================================================
        print_separator("Step 5: Examining the DAG structure")

        print("The conversation now has a branching structure:")
        print("- First question about Python vs Rust")
        print("- Branch 1: Web server recommendation")
        print("- Branch 2: Data science recommendation (forked)")
        print()

        print_dag_structure(client, dag_id)

        # ============================================================
        # Summary
        # ============================================================
        print_separator("Demo Complete")

        print("This example demonstrated:")
        print("  1. Starting a streaming conversation")
        print("  2. Continuing an existing conversation")
        print("  3. Forking from an earlier node to explore alternatives")
        print("  4. Listing all DAGs")
        print("  5. Examining the branching structure of a conversation")
        print()
        print("The DAG structure allows you to:")
        print("  - Track conversation history")
        print("  - Branch at any point to explore alternatives")
        print("  - Maintain multiple conversation paths")
        print("  - Resume from any node")
        print()


if __name__ == "__main__":
    main()
