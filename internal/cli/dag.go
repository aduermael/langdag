package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/langdag/langdag/internal/config"
	"github.com/langdag/langdag/pkg/types"
	"github.com/spf13/cobra"
)

// dagCmd is the parent command for DAG instance operations.
var dagCmd = &cobra.Command{
	Use:   "dag",
	Short: "Manage DAG instances",
	Long:  `Commands for managing DAG instances - created from workflows or chat sessions.`,
}

// dagListCmd lists all DAG instances.
var dagListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all DAG instances",
	Long:  `List all DAG instances, whether created from workflows or chat sessions.`,
	Run:   runDAGList,
}

// dagShowCmd shows a DAG instance.
var dagShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show DAG instance details",
	Long:  `Show the details and node history of a DAG instance.`,
	Args:  cobra.ExactArgs(1),
	Run:   runDAGShow,
}

// dagDeleteCmd deletes a DAG instance.
var dagDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a DAG instance",
	Long:  `Delete a DAG instance and all its nodes.`,
	Args:  cobra.ExactArgs(1),
	Run:   runDAGDelete,
}

func init() {
	dagCmd.AddCommand(dagListCmd)
	dagCmd.AddCommand(dagShowCmd)
	dagCmd.AddCommand(dagDeleteCmd)
}

func runDAGList(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		exitError("failed to load config: %v", err)
	}

	// Initialize storage
	store, err := initStorage(ctx, cfg)
	if err != nil {
		exitError("%v", err)
	}
	defer store.Close()

	// List DAGs
	dags, err := store.ListDAGs(ctx)
	if err != nil {
		exitError("failed to list DAGs: %v", err)
	}

	if len(dags) == 0 {
		fmt.Println("No DAGs found.")
		return
	}

	fmt.Printf("DAGs (%d):\n\n", len(dags))
	for _, dag := range dags {
		title := dag.Title
		if title == "" {
			title = "(untitled)"
		}

		// Show source indicator
		source := "chat"
		if dag.WorkflowID != "" {
			source = "workflow"
		}

		fmt.Printf("  %s  %s [%s]\n", dag.ID[:8], title, source)
		fmt.Printf("    Status: %s", dag.Status)
		if dag.Model != "" {
			fmt.Printf(", Model: %s", dag.Model)
		}
		fmt.Printf(", Created: %s\n", dag.CreatedAt.Format("2006-01-02 15:04"))
		fmt.Println()
	}
}

func runDAGShow(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	dagID := args[0]

	// Load config
	cfg, err := config.Load()
	if err != nil {
		exitError("failed to load config: %v", err)
	}

	// Initialize storage
	store, err := initStorage(ctx, cfg)
	if err != nil {
		exitError("%v", err)
	}
	defer store.Close()

	// Get DAG - try full ID first, then partial match
	dag, err := store.GetDAG(ctx, dagID)
	if err != nil {
		exitError("failed to get DAG: %v", err)
	}
	if dag == nil {
		// Try partial ID match
		dags, err := store.ListDAGs(ctx)
		if err != nil {
			exitError("failed to list DAGs: %v", err)
		}
		for _, d := range dags {
			if strings.HasPrefix(d.ID, dagID) {
				dag = d
				break
			}
		}
	}
	if dag == nil {
		exitError("DAG not found: %s", dagID)
	}

	// Get nodes
	nodes, err := store.GetDAGNodes(ctx, dag.ID)
	if err != nil {
		exitError("failed to get nodes: %v", err)
	}

	// Print DAG info
	fmt.Printf("DAG: %s\n", dag.ID)
	if dag.Title != "" {
		fmt.Printf("Title: %s\n", dag.Title)
	}

	// Show source
	if dag.WorkflowID != "" {
		fmt.Printf("Source: workflow (%s)\n", dag.WorkflowID)
	} else {
		fmt.Printf("Source: chat\n")
	}

	fmt.Printf("Status: %s\n", dag.Status)
	if dag.Model != "" {
		fmt.Printf("Model: %s\n", dag.Model)
	}
	if dag.SystemPrompt != "" {
		fmt.Printf("System: %s\n", truncate(dag.SystemPrompt, 60))
	}
	fmt.Printf("Created: %s\n", dag.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Nodes: %d\n", len(nodes))

	// Show input/output for workflow DAGs
	if dag.Input != nil && len(dag.Input) > 0 && string(dag.Input) != "{}" {
		fmt.Printf("Input: %s\n", truncate(string(dag.Input), 60))
	}
	if dag.Output != nil && len(dag.Output) > 0 {
		fmt.Printf("Output: %s\n", truncate(string(dag.Output), 60))
	}

	fmt.Println()

	// Print nodes as tree
	if len(nodes) > 0 {
		fmt.Println("Node history:")
		for i, node := range nodes {
			prefix := "├─"
			if i == len(nodes)-1 {
				prefix = "└─"
			}
			fmt.Printf("%s ", prefix)
			printDAGNodeCompact(node)
		}
	}
}

func runDAGDelete(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	dagID := args[0]

	// Load config
	cfg, err := config.Load()
	if err != nil {
		exitError("failed to load config: %v", err)
	}

	// Initialize storage
	store, err := initStorage(ctx, cfg)
	if err != nil {
		exitError("%v", err)
	}
	defer store.Close()

	// Get DAG to verify it exists - try full ID first, then partial match
	dag, err := store.GetDAG(ctx, dagID)
	if err != nil {
		exitError("failed to get DAG: %v", err)
	}
	if dag == nil {
		// Try partial ID match
		dags, err := store.ListDAGs(ctx)
		if err != nil {
			exitError("failed to list DAGs: %v", err)
		}
		for _, d := range dags {
			if strings.HasPrefix(d.ID, dagID) {
				dag = d
				break
			}
		}
	}
	if dag == nil {
		exitError("DAG not found: %s", dagID)
	}

	// Delete DAG
	if err := store.DeleteDAG(ctx, dag.ID); err != nil {
		exitError("failed to delete DAG: %v", err)
	}

	title := dag.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Printf("Deleted DAG: %s (%s)\n", dag.ID[:8], title)
}

func printDAGNodeCompact(node *types.DAGNode) {
	var content string
	if err := json.Unmarshal(node.Content, &content); err != nil {
		content = string(node.Content)
	}

	role := string(node.NodeType)

	// Truncate long content
	if len(content) > 60 {
		content = content[:57] + "..."
	}

	content = strings.ReplaceAll(content, "\n", " ")

	// Build info string
	var info []string
	if node.Status != "" {
		info = append(info, string(node.Status))
	}
	if node.TokensIn > 0 || node.TokensOut > 0 {
		info = append(info, fmt.Sprintf("tokens: %d/%d", node.TokensIn, node.TokensOut))
	}
	if node.LatencyMs > 0 {
		info = append(info, fmt.Sprintf("%dms", node.LatencyMs))
	}

	infoStr := ""
	if len(info) > 0 {
		infoStr = " (" + strings.Join(info, ", ") + ")"
	}

	fmt.Printf("%s [%s]: %s%s\n", node.ID[:8], role, content, infoStr)
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
