package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/langdag/langdag/internal/config"
	"github.com/langdag/langdag/pkg/types"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// lsCmd lists all DAG instances.
var lsCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List all DAG instances",
	Long:    `List all DAG instances, whether created from workflows or chat sessions.`,
	Run:     runDAGList,
}

// showCmd shows a DAG instance.
var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show DAG instance details",
	Long:  `Show the details and node history of a DAG instance.`,
	Args:  cobra.ExactArgs(1),
	Run:   runDAGShow,
}

// rmCmd deletes a DAG instance.
var rmCmd = &cobra.Command{
	Use:     "rm <id>",
	Aliases: []string{"delete"},
	Short:   "Delete a DAG instance",
	Long:    `Delete a DAG instance and all its nodes.`,
	Args:    cobra.ExactArgs(1),
	Run:     runDAGDelete,
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
		if outputJSON {
			fmt.Println("[]")
		} else if outputYAML {
			fmt.Println("[]")
		} else {
			fmt.Println("No DAGs found.")
		}
		return
	}

	// Handle JSON/YAML output
	if printFormatted(dags) {
		return
	}

	// Default: table output
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Title", "Source", "Status", "Model", "Created"})
	table.SetBorder(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	for _, dag := range dags {
		title := dag.Title
		if title == "" {
			title = "(untitled)"
		}
		if len(title) > 20 {
			title = title[:17] + "..."
		}

		source := "chat"
		if dag.WorkflowID != "" {
			source = "workflow"
		}

		model := dag.Model
		if len(model) > 30 {
			model = model[:27] + "..."
		}

		table.Append([]string{
			dag.ID[:8],
			title,
			source,
			string(dag.Status),
			model,
			dag.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	table.Render()
}

// DAGWithNodes is used for JSON/YAML output of a DAG with its nodes.
type DAGWithNodes struct {
	*types.DAG `yaml:",inline"`
	Nodes      []*types.DAGNode `json:"nodes" yaml:"nodes"`
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

	// Handle JSON/YAML output
	if outputJSON || outputYAML {
		output := DAGWithNodes{
			DAG:   dag,
			Nodes: nodes,
		}
		printFormatted(output)
		return
	}

	// Default: structured text output
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

	// Print nodes as a tree
	if len(nodes) > 0 {
		fmt.Println("Node history:")
		printDAGTree(nodes)
	}
}

// printDAGTree prints nodes as a tree structure
func printDAGTree(nodes []*types.DAGNode) {
	if len(nodes) == 0 {
		return
	}

	// Build parent -> children map
	childrenMap := make(map[string][]*types.DAGNode)
	var roots []*types.DAGNode

	for _, node := range nodes {
		if node.ParentID == "" {
			roots = append(roots, node)
		} else {
			childrenMap[node.ParentID] = append(childrenMap[node.ParentID], node)
		}
	}

	// Print tree recursively
	for i, root := range roots {
		isLast := i == len(roots)-1
		printNodeTree(root, "", isLast, childrenMap)
	}
}

// printNodeTree recursively prints a node and its children
func printNodeTree(node *types.DAGNode, prefix string, isLast bool, childrenMap map[string][]*types.DAGNode) {
	// Determine connector
	connector := "├─"
	if isLast {
		connector = "└─"
	}

	fmt.Printf("%s%s ", prefix, connector)
	printDAGNodeCompact(node)

	// Determine prefix for children
	childPrefix := prefix + "│  "
	if isLast {
		childPrefix = prefix + "   "
	}

	// Print children
	children := childrenMap[node.ID]
	for i, child := range children {
		childIsLast := i == len(children)-1
		printNodeTree(child, childPrefix, childIsLast, childrenMap)
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
