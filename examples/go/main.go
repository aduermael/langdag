// Example demonstrating LangDAG Go SDK usage.
//
// This example shows:
// - Starting a chat conversation with streaming
// - Continuing the conversation
// - Forking from an earlier node to explore alternatives
// - Listing DAGs and viewing branching structure
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	langdag "github.com/langdag/langdag-go"
)

func main() {
	// Get server URL from environment or use default
	baseURL := os.Getenv("LANGDAG_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	// Create client with optional API key
	opts := []langdag.Option{}
	if apiKey := os.Getenv("LANGDAG_API_KEY"); apiKey != "" {
		opts = append(opts, langdag.WithAPIKey(apiKey))
	}

	client := langdag.NewClient(baseURL, opts...)
	ctx := context.Background()

	fmt.Println("========================================")
	fmt.Println("LangDAG Go SDK Example")
	fmt.Println("========================================")
	fmt.Println()

	// Check server health
	fmt.Println("[1] Checking server health...")
	health, err := client.Health(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	fmt.Printf("    Server status: %s\n", health.Status)
	fmt.Println()

	// ========================================
	// STEP 1: Start a new chat with streaming
	// ========================================
	fmt.Println("[2] Starting a new chat about programming languages...")
	fmt.Println("    Question: What are the key differences between Go and Rust?")
	fmt.Println()

	var dagID string
	var firstNodeID string
	var fullResponse strings.Builder

	err = client.ChatStream(ctx, &langdag.NewChatRequest{
		Message:      "What are the key differences between Go and Rust? Give me a brief overview.",
		SystemPrompt: "You are a helpful programming assistant. Keep responses concise.",
	}, func(event langdag.SSEEvent) error {
		switch event.Type {
		case langdag.SSEEventStart:
			dagID = event.DAGID
			fmt.Printf("    [Stream started] DAG ID: %s\n", shortID(dagID))
		case langdag.SSEEventDelta:
			fullResponse.WriteString(event.Content)
			// Print dots to show streaming progress
			fmt.Print(".")
		case langdag.SSEEventDone:
			firstNodeID = event.NodeID
			fmt.Println()
			fmt.Printf("    [Stream complete] Node ID: %s\n", shortID(firstNodeID))
		case langdag.SSEEventError:
			return fmt.Errorf("stream error: %s", event.Error)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Chat failed: %v", err)
	}

	fmt.Println()
	fmt.Println("    Response preview:")
	fmt.Printf("    %s...\n", truncate(fullResponse.String(), 200))
	fmt.Println()

	// ========================================
	// STEP 2: Continue the conversation
	// ========================================
	fmt.Println("[3] Continuing the conversation...")
	fmt.Println("    Follow-up: Which one is better for web services?")
	fmt.Println()

	var secondNodeID string
	fullResponse.Reset()

	err = client.ContinueChatStream(ctx, dagID, &langdag.ContinueChatRequest{
		Message: "Which one would you recommend for building web services and why?",
	}, func(event langdag.SSEEvent) error {
		switch event.Type {
		case langdag.SSEEventStart:
			fmt.Printf("    [Continuing DAG: %s]\n", shortID(dagID))
		case langdag.SSEEventDelta:
			fullResponse.WriteString(event.Content)
			fmt.Print(".")
		case langdag.SSEEventDone:
			secondNodeID = event.NodeID
			fmt.Println()
			fmt.Printf("    [Stream complete] Node ID: %s\n", shortID(secondNodeID))
		case langdag.SSEEventError:
			return fmt.Errorf("stream error: %s", event.Error)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Continue chat failed: %v", err)
	}

	fmt.Println()
	fmt.Println("    Response preview:")
	fmt.Printf("    %s...\n", truncate(fullResponse.String(), 200))
	fmt.Println()

	// ========================================
	// STEP 3: Fork from the first response
	// ========================================
	fmt.Println("[4] Forking from the first response to explore an alternative...")
	fmt.Println("    Alternative question: What about for systems programming?")
	fmt.Println()

	var forkNodeID string
	fullResponse.Reset()

	err = client.ForkChatStream(ctx, dagID, &langdag.ForkChatRequest{
		NodeID:  firstNodeID,
		Message: "What about for systems programming and embedded development?",
	}, func(event langdag.SSEEvent) error {
		switch event.Type {
		case langdag.SSEEventStart:
			fmt.Printf("    [Forking from node: %s]\n", shortID(firstNodeID))
		case langdag.SSEEventDelta:
			fullResponse.WriteString(event.Content)
			fmt.Print(".")
		case langdag.SSEEventDone:
			forkNodeID = event.NodeID
			fmt.Println()
			fmt.Printf("    [Fork complete] New node ID: %s\n", shortID(forkNodeID))
		case langdag.SSEEventError:
			return fmt.Errorf("stream error: %s", event.Error)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Fork chat failed: %v", err)
	}

	fmt.Println()
	fmt.Println("    Response preview:")
	fmt.Printf("    %s...\n", truncate(fullResponse.String(), 200))
	fmt.Println()

	// ========================================
	// STEP 4: List all DAGs
	// ========================================
	fmt.Println("[5] Listing all DAGs...")
	fmt.Println()

	dags, err := client.ListDAGs(ctx)
	if err != nil {
		log.Fatalf("Failed to list DAGs: %v", err)
	}

	fmt.Printf("    Found %d DAG(s):\n", len(dags))
	for i, dag := range dags {
		if i >= 5 {
			fmt.Printf("    ... and %d more\n", len(dags)-5)
			break
		}
		title := dag.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("    - %s: %s [%s]\n", shortID(dag.ID), title, dag.Status)
	}
	fmt.Println()

	// ========================================
	// STEP 5: Show DAG structure with branches
	// ========================================
	fmt.Println("[6] Showing DAG structure with branches...")
	fmt.Println()

	dagDetail, err := client.GetDAG(ctx, dagID)
	if err != nil {
		log.Fatalf("Failed to get DAG: %v", err)
	}

	fmt.Printf("    DAG ID: %s\n", shortID(dagDetail.ID))
	fmt.Printf("    Status: %s\n", dagDetail.Status)
	fmt.Printf("    Nodes:  %d\n", len(dagDetail.Nodes))
	fmt.Println()
	fmt.Println("    Node structure:")

	// Build a simple tree visualization
	printNodeTree(dagDetail.Nodes)

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Example complete!")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  - Created DAG: %s\n", shortID(dagID))
	fmt.Printf("  - Main branch: question -> response -> follow-up -> response\n")
	fmt.Printf("  - Fork branch: diverged after first response for systems programming question\n")
}

// shortID returns the first 8 characters of an ID for display
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	// Remove newlines for cleaner display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// printNodeTree prints a simple visualization of the node structure
func printNodeTree(nodes []langdag.Node) {
	if len(nodes) == 0 {
		fmt.Println("    (no nodes)")
		return
	}

	// Build parent-child relationships
	children := make(map[string][]langdag.Node)
	var roots []langdag.Node

	for _, node := range nodes {
		if node.ParentID == "" {
			roots = append(roots, node)
		} else {
			children[node.ParentID] = append(children[node.ParentID], node)
		}
	}

	// Print tree starting from roots
	for _, root := range roots {
		printNode(root, children, "", true)
	}
}

// printNode recursively prints a node and its children
func printNode(node langdag.Node, children map[string][]langdag.Node, prefix string, isLast bool) {
	// Choose the connector
	connector := "|-- "
	if isLast {
		connector = "`-- "
	}

	// Format node info
	contentPreview := truncate(node.Content, 40)
	if contentPreview == "" {
		contentPreview = "(empty)"
	}

	fmt.Printf("    %s%s[%s] %s: %s\n", prefix, connector, shortID(node.ID), node.NodeType, contentPreview)

	// Calculate new prefix for children
	newPrefix := prefix
	if isLast {
		newPrefix += "    "
	} else {
		newPrefix += "|   "
	}

	// Print children
	nodeChildren := children[node.ID]
	for i, child := range nodeChildren {
		isLastChild := i == len(nodeChildren)-1
		printNode(child, children, newPrefix, isLastChild)
	}
}
