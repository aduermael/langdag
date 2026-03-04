package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/langdag/langdag/pkg/types"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// lsCmd lists all root nodes (conversations).
var lsCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List all conversations",
	Long:    `List all root nodes (conversations).`,
	Run:     runNodeList,
}

// showCmd shows a node tree.
var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show node tree",
	Long:  `Show the details and node tree from a given node.`,
	Args:  cobra.ExactArgs(1),
	Run:   runNodeShow,
}

// rmCmd deletes a node and its subtree.
var rmCmd = &cobra.Command{
	Use:     "rm <id>",
	Aliases: []string{"delete"},
	Short:   "Delete a node and its subtree",
	Long:    `Delete a node and all its descendant nodes.`,
	Args:    cobra.ExactArgs(1),
	Run:     runNodeDelete,
}

func runNodeList(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	client, err := newLibraryClient(ctx)
	if err != nil {
		exitError("%v", err)
	}
	defer client.Close()

	roots, err := client.ListConversations(ctx)
	if err != nil {
		exitError("failed to list nodes: %v", err)
	}

	if len(roots) == 0 {
		if outputJSON || outputYAML {
			fmt.Println("[]")
		} else {
			fmt.Println("No conversations found.")
		}
		return
	}

	if printFormatted(roots) {
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Title", "Model", "Status", "Created"})
	table.SetBorder(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	for _, node := range roots {
		title := node.Title
		if title == "" {
			title = "(untitled)"
		}
		if len(title) > 30 {
			title = title[:27] + "..."
		}

		model := node.Model
		if len(model) > 30 {
			model = model[:27] + "..."
		}

		table.Append([]string{
			node.ID[:8],
			title,
			model,
			node.Status,
			node.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	table.Render()
}

func runNodeShow(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	nodeID := args[0]

	client, err := newLibraryClient(ctx)
	if err != nil {
		exitError("%v", err)
	}
	defer client.Close()

	node, err := client.GetNode(ctx, nodeID)
	if err != nil {
		exitError("failed to get node: %v", err)
	}
	if node == nil {
		exitError("node not found: %s", nodeID)
	}

	// Get subtree
	nodes, err := client.GetSubtree(ctx, node.ID)
	if err != nil {
		exitError("failed to get tree: %v", err)
	}

	if outputJSON || outputYAML {
		printFormatted(nodes)
		return
	}

	fmt.Printf("Node: %s\n", node.ID)
	if node.Title != "" {
		fmt.Printf("Title: %s\n", node.Title)
	}
	if node.SystemPrompt != "" {
		fmt.Printf("System: %s\n", truncate(node.SystemPrompt, 60))
	}
	fmt.Printf("Created: %s\n", node.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Nodes: %d\n", len(nodes))

	if len(nodes) > 0 {
		// If showing a non-root node, show root and skipped ancestors
		if node.ParentID != "" {
			ancestors, err := client.GetAncestors(ctx, node.ID)
			if err == nil && len(ancestors) > 1 {
				// ancestors is root-first and includes the node itself
				root := ancestors[0]
				fmt.Printf("├─ %s (root)\n", root.ID[:8])
				// Skipped nodes = ancestors minus root and the target node
				skipped := len(ancestors) - 2
				if skipped > 0 {
					if skipped == 1 {
						fmt.Printf("├─ ... (1 node)\n")
					} else {
						fmt.Printf("├─ ... (%d nodes)\n", skipped)
					}
				}
			}
		}
		printNodeTree(nodes, node.ID, node.ID)
	}
}

// printNodeTree prints nodes as a tree structure.
// rootID is the ID of the node to treat as the tree root.
// highlightID is the node whose ID should be displayed in bold.
func printNodeTree(nodes []*types.Node, rootID, highlightID string) {
	if len(nodes) == 0 {
		return
	}

	childrenMap := make(map[string][]*types.Node)
	var roots []*types.Node

	for _, node := range nodes {
		if node.ID == rootID {
			roots = append(roots, node)
		} else {
			childrenMap[node.ParentID] = append(childrenMap[node.ParentID], node)
		}
	}

	var printChain func(node *types.Node, prefix string, hasMoreSiblings bool)
	printChain = func(node *types.Node, prefix string, hasMoreSiblings bool) {
		children := childrenMap[node.ID]
		isLeaf := len(children) == 0
		isBranchPoint := len(children) > 1

		var connector string
		if isLeaf || isBranchPoint {
			if hasMoreSiblings {
				connector = "│└─"
			} else {
				connector = "└─"
			}
		} else {
			if hasMoreSiblings {
				connector = "│├─"
			} else {
				connector = "├─"
			}
		}

		fmt.Printf("%s%s ", prefix, connector)
		printNodeCompact(node, node.ID == highlightID)

		if isLeaf {
			return
		}

		if isBranchPoint {
			childPrefix := prefix + " "
			for i, child := range children {
				childHasMoreSiblings := i < len(children)-1
				printChain(child, childPrefix, childHasMoreSiblings)
			}
		} else {
			printChain(children[0], prefix, hasMoreSiblings)
		}
	}

	for _, root := range roots {
		printChain(root, "", false)
	}
}

func runNodeDelete(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	nodeID := args[0]

	client, err := newLibraryClient(ctx)
	if err != nil {
		exitError("%v", err)
	}
	defer client.Close()

	node, err := client.GetNode(ctx, nodeID)
	if err != nil {
		exitError("failed to get node: %v", err)
	}
	if node == nil {
		exitError("node not found: %s", nodeID)
	}

	if err := client.DeleteNode(ctx, node.ID); err != nil {
		exitError("failed to delete node: %v", err)
	}

	title := node.Title
	if title == "" {
		title = truncate(node.Content, 30)
	}
	fmt.Printf("Deleted node: %s (%s)\n", node.ID[:8], title)
}

func printNodeCompact(node *types.Node, bold bool) {
	content := node.Content
	role := string(node.NodeType)

	if len(content) > 60 {
		content = content[:57] + "..."
	}
	content = strings.ReplaceAll(content, "\n", " ")

	var info []string
	if node.Status != "" {
		info = append(info, node.Status)
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

	id := node.ID[:8]
	if bold {
		id = "\033[1m" + id + "\033[0m"
	}

	fmt.Printf("%s [%s]: %s%s\n", id, role, content, infoStr)
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
