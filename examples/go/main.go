// Example demonstrating LangDAG Go SDK usage.
//
// This example shows:
// - Starting a new conversation with Prompt
// - Streaming a response with PromptStream
// - Continuing from a node with node.Prompt()
// - Branching from an earlier node
// - Listing roots and viewing tree structure
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
	baseURL := os.Getenv("LANGDAG_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

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
	// STEP 1: Start a new conversation with streaming
	// ========================================
	fmt.Println("[2] Starting a new conversation (streaming)...")
	fmt.Println("    Question: What are the key differences between Go and Rust?")
	fmt.Println()

	stream, err := client.PromptStream(ctx, "What are the key differences between Go and Rust? Give me a brief overview.",
		langdag.WithSystem("You are a helpful programming assistant. Keep responses concise."),
	)
	if err != nil {
		log.Fatalf("PromptStream failed: %v", err)
	}

	var fullResponse strings.Builder
	for event := range stream.Events() {
		if event.Type == "delta" {
			fullResponse.WriteString(event.Content)
			fmt.Print(".")
		}
	}
	fmt.Println()

	firstNode, err := stream.Node()
	if err != nil {
		log.Fatalf("stream.Node() failed: %v", err)
	}
	fmt.Printf("    [Stream complete] Node ID: %s\n", shortID(firstNode.ID))
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

	secondNode, err := firstNode.Prompt(ctx, "Which one would you recommend for building web services and why?")
	if err != nil {
		log.Fatalf("node.Prompt failed: %v", err)
	}
	fmt.Printf("    [Response] Node ID: %s\n", shortID(secondNode.ID))
	fmt.Printf("    %s...\n", truncate(secondNode.Content, 200))
	fmt.Println()

	// ========================================
	// STEP 3: Branch from the first response
	// ========================================
	fmt.Println("[4] Branching from the first response...")
	fmt.Println("    Alternative: What about for systems programming?")
	fmt.Println()

	branchNode, err := firstNode.Prompt(ctx, "What about for systems programming and embedded development?")
	if err != nil {
		log.Fatalf("branch prompt failed: %v", err)
	}
	fmt.Printf("    [Branch] Node ID: %s\n", shortID(branchNode.ID))
	fmt.Printf("    %s...\n", truncate(branchNode.Content, 200))
	fmt.Println()

	// ========================================
	// STEP 4: List all conversation roots
	// ========================================
	fmt.Println("[5] Listing all conversation roots...")
	fmt.Println()

	roots, err := client.ListRoots(ctx)
	if err != nil {
		log.Fatalf("ListRoots failed: %v", err)
	}

	fmt.Printf("    Found %d root(s):\n", len(roots))
	for i, r := range roots {
		if i >= 5 {
			fmt.Printf("    ... and %d more\n", len(roots)-5)
			break
		}
		title := r.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("    - %s: %s\n", shortID(r.ID), title)
	}
	fmt.Println()

	// ========================================
	// STEP 5: Show tree structure
	// ========================================
	fmt.Println("[6] Showing tree structure...")
	fmt.Println()

	// Find our root (it's the one we just created)
	if len(roots) > 0 {
		tree, err := client.GetTree(ctx, roots[len(roots)-1].ID)
		if err != nil {
			log.Fatalf("GetTree failed: %v", err)
		}

		fmt.Printf("    Tree has %d nodes:\n", len(tree.Nodes))
		printNodeTree(tree.Nodes)
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Example complete!")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  - Main branch: question -> response -> follow-up -> response\n")
	fmt.Printf("  - Fork branch: diverged after first response for systems programming question\n")
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func printNodeTree(nodes []langdag.Node) {
	if len(nodes) == 0 {
		fmt.Println("    (no nodes)")
		return
	}

	children := make(map[string][]langdag.Node)
	var roots []langdag.Node

	for _, node := range nodes {
		if node.ParentID == "" {
			roots = append(roots, node)
		} else {
			children[node.ParentID] = append(children[node.ParentID], node)
		}
	}

	for _, root := range roots {
		printNode(root, children, "", true)
	}
}

func printNode(node langdag.Node, children map[string][]langdag.Node, prefix string, isLast bool) {
	connector := "|-- "
	if isLast {
		connector = "`-- "
	}

	contentPreview := truncate(node.Content, 40)
	if contentPreview == "" {
		contentPreview = "(empty)"
	}

	fmt.Printf("    %s%s[%s] %s: %s\n", prefix, connector, shortID(node.ID), node.Type, contentPreview)

	newPrefix := prefix
	if isLast {
		newPrefix += "    "
	} else {
		newPrefix += "|   "
	}

	nodeChildren := children[node.ID]
	for i, child := range nodeChildren {
		isLastChild := i == len(nodeChildren)-1
		printNode(child, children, newPrefix, isLastChild)
	}
}
