#!/usr/bin/env python3
"""LangDAG Python SDK Example.

This example demonstrates the key features of the LangDAG SDK:
1. Starting a conversation with streaming
2. Continuing a conversation from a node
3. Branching from an earlier node to explore alternatives
4. Listing root nodes and exploring the conversation tree

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


def print_streaming_response(events) -> tuple[str, str]:
    """Print streaming response and return node_id and full content."""
    node_id = ""
    content_parts = []

    print("Assistant: ", end="", flush=True)

    for event in events:
        if event.event == SSEEventType.DELTA:
            if event.content:
                print(event.content, end="", flush=True)
                content_parts.append(event.content)
        elif event.event == SSEEventType.DONE:
            node_id = event.node_id or ""
        elif event.event == SSEEventType.ERROR:
            print(f"\n[Error: {event.data}]")

    print("\n")  # End the response line
    return node_id, "".join(content_parts)


def print_tree_structure(client: LangDAGClient, root_id: str) -> None:
    """Print the tree structure of a conversation."""
    tree = client.get_tree(root_id)

    # Get root node info
    root = None
    for node in tree:
        if node.parent_id is None:
            root = node
            break

    if root:
        print(f"Root: {root.id[:8]}...")
        print(f"  Title: {root.title or '(untitled)'}")
    print(f"  Nodes: {len(tree)}")
    print()

    # Build parent-child relationships for visualization
    nodes_by_id = {node.id: node for node in tree}
    children: dict[str | None, list[str]] = {}

    for node in tree:
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
    for root_id_key in children.get(None, []):
        print_node(root_id_key, indent=2)


def main() -> None:
    """Run the LangDAG SDK demonstration."""
    print("\n" + "#" * 60)
    print("#  LangDAG Python SDK Demo")
    print("#  Demonstrating conversation branching and tree exploration")
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
        print_separator("Step 1: Starting a new conversation (streaming)")

        print("User: What are the main differences between Python and Rust?\n")

        events = client.prompt(
            message="What are the main differences between Python and Rust? Keep your answer brief.",
            stream=True,
        )

        first_response_node, _ = print_streaming_response(events)

        print(f"[Response node: {first_response_node[:8]}...]")

        # ============================================================
        # Step 2: Continue the conversation from the response node
        # ============================================================
        print_separator("Step 2: Continuing the conversation")

        print("User: Which one would you recommend for building a web server?\n")

        events = client.prompt_from(
            node_id=first_response_node,
            message="Which one would you recommend for building a web server?",
            stream=True,
        )

        second_response_node, _ = print_streaming_response(events)

        print(f"[Continued conversation - Node: {second_response_node[:8]}...]")

        # ============================================================
        # Step 3: Branch from the first response to explore alternatives
        # ============================================================
        print_separator("Step 3: Branching to explore an alternative path")

        print(f"[Branching from node {first_response_node[:8]}... to ask a different question]")
        print()
        print("User: Which one would you recommend for data science work?\n")

        events = client.prompt_from(
            node_id=first_response_node,
            message="Which one would you recommend for data science work?",
            stream=True,
        )

        branch_response_node, _ = print_streaming_response(events)

        print(f"[Branched conversation - New node: {branch_response_node[:8]}...]")

        # ============================================================
        # Step 4: List all root nodes
        # ============================================================
        print_separator("Step 4: Listing all root nodes")

        roots = client.list_roots()
        print(f"Found {len(roots)} conversation(s):\n")

        for root in roots[:5]:  # Show first 5
            print(f"  - {root.id[:8]}... | {root.title or '(untitled)'}")

        if len(roots) > 5:
            print(f"  ... and {len(roots) - 5} more")

        # ============================================================
        # Step 5: Show the branching tree structure
        # ============================================================
        print_separator("Step 5: Examining the tree structure")

        print("The conversation now has a branching structure:")
        print("- First question about Python vs Rust")
        print("- Branch 1: Web server recommendation")
        print("- Branch 2: Data science recommendation (branched)")
        print()

        # Find the root of our conversation by traversing up
        tree = client.get_tree(first_response_node)
        root_id = None
        for node in tree:
            if node.parent_id is None:
                root_id = node.id
                break

        if root_id:
            print_tree_structure(client, root_id)

        # ============================================================
        # Summary
        # ============================================================
        print_separator("Demo Complete")

        print("This example demonstrated:")
        print("  1. Starting a streaming conversation with prompt()")
        print("  2. Continuing from a node with prompt_from()")
        print("  3. Branching from an earlier node to explore alternatives")
        print("  4. Listing all root nodes with list_roots()")
        print("  5. Examining the tree structure with get_tree()")
        print()
        print("The node-centric API allows you to:")
        print("  - Track conversation history as a tree")
        print("  - Branch at any point to explore alternatives")
        print("  - Maintain multiple conversation paths")
        print("  - Resume from any node")
        print()


if __name__ == "__main__":
    main()
