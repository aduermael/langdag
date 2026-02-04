package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/langdag/langdag/internal/config"
	"github.com/langdag/langdag/internal/conversation"
	"github.com/langdag/langdag/internal/provider/anthropic"
	"github.com/langdag/langdag/internal/storage/sqlite"
	"github.com/langdag/langdag/pkg/types"
	"github.com/spf13/cobra"
)

var (
	chatModel        string
	chatSystemPrompt string
	chatNodeID       string
)

// chatCmd is the parent command for chat operations.
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Interactive chat sessions",
	Long:  `Commands for interactive chat - start new conversations or continue existing DAGs.`,
}

// chatNewCmd starts a new conversation.
var chatNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a new conversation",
	Long:  `Start a new interactive conversation with an LLM. Creates a new DAG with human as the first node.`,
	Run:   runChatNew,
}

// chatContinueCmd continues an existing DAG.
var chatContinueCmd = &cobra.Command{
	Use:   "continue [dag-id]",
	Short: "Continue an existing DAG",
	Long: `Continue an existing DAG by attaching a human node. Works with any DAG - from chat or workflow.

Use --node to continue from a specific node, creating a new branch in the conversation.`,
	Args: cobra.MaximumNArgs(1),
	Run:  runChatContinue,
}

func init() {
	chatCmd.AddCommand(chatNewCmd)
	chatCmd.AddCommand(chatContinueCmd)

	chatNewCmd.Flags().StringVarP(&chatModel, "model", "m", "claude-sonnet-4-20250514", "model to use")
	chatNewCmd.Flags().StringVarP(&chatSystemPrompt, "system", "s", "", "system prompt")

	chatContinueCmd.Flags().StringVarP(&chatNodeID, "node", "n", "", "continue from a specific node ID (creates a new branch)")
}

// initStorage initializes the SQLite storage from config.
func initStorage(ctx context.Context, cfg *config.Config) (*sqlite.SQLiteStorage, error) {
	storagePath := cfg.Storage.Path
	if storagePath == "./langdag.db" {
		storagePath = config.GetDefaultStoragePath()
	}

	// Ensure storage directory exists
	if err := config.EnsureStorageDir(storagePath); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	store, err := sqlite.New(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open storage: %w", err)
	}

	if err := store.Init(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	return store, nil
}

func runChatNew(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		exitError("failed to load config: %v", err)
	}

	// Get API key
	apiKey := cfg.Providers.Anthropic.APIKey
	if apiKey == "" {
		exitError("ANTHROPIC_API_KEY not set")
	}

	// Initialize storage
	store, err := initStorage(ctx, cfg)
	if err != nil {
		exitError("%v", err)
	}
	defer store.Close()

	// Create provider
	prov := anthropic.New(apiKey)

	// Create conversation manager
	mgr := conversation.NewManager(store, prov)

	// Create new DAG for conversation
	dag, err := mgr.CreateDAG(ctx, chatModel, chatSystemPrompt, "")
	if err != nil {
		exitError("failed to create DAG: %v", err)
	}

	fmt.Printf("Starting new conversation (id: %s)\n", dag.ID)
	if chatSystemPrompt != "" {
		fmt.Printf("System prompt: %s\n", chatSystemPrompt)
	}
	fmt.Println()

	// Run interactive loop
	runInteractiveChat(ctx, mgr, dag)
}

func runChatContinue(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Validate arguments
	if chatNodeID == "" && len(args) == 0 {
		exitError("requires either a dag-id argument or --node flag")
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		exitError("failed to load config: %v", err)
	}

	// Get API key
	apiKey := cfg.Providers.Anthropic.APIKey
	if apiKey == "" {
		exitError("ANTHROPIC_API_KEY not set")
	}

	// Initialize storage
	store, err := initStorage(ctx, cfg)
	if err != nil {
		exitError("%v", err)
	}
	defer store.Close()

	// Create provider
	prov := anthropic.New(apiKey)

	// Create conversation manager
	mgr := conversation.NewManager(store, prov)

	var dag *types.DAG
	var currentNodeID string

	if chatNodeID != "" {
		// Continue from a specific node
		node, nodeDag, err := mgr.GetNodeWithDAG(ctx, chatNodeID)
		if err != nil {
			exitError("failed to get node: %v", err)
		}
		dag = nodeDag
		currentNodeID = node.ID

		fmt.Printf("Continuing from node %s\n", node.ID[:8])
		fmt.Printf("DAG: %s\n", dag.ID)
	} else {
		// Continue existing DAG from the end
		dagID := args[0]

		// Get DAG - try full ID first, then partial match
		dag, err = mgr.GetDAG(ctx, dagID)
		if err != nil {
			exitError("failed to get DAG: %v", err)
		}
		if dag == nil {
			// Try partial ID match
			dags, err := mgr.ListDAGs(ctx)
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

		fmt.Printf("Continuing DAG: %s\n", dag.ID)
	}

	if dag.Title != "" {
		fmt.Printf("Title: %s\n", dag.Title)
	}
	if dag.WorkflowID != "" {
		fmt.Printf("From workflow: %s\n", dag.WorkflowID)
	}
	fmt.Println()

	// Show recent history (walk up from current node if set, otherwise show last nodes)
	nodes, err := mgr.GetNodes(ctx, dag.ID)
	if err == nil && len(nodes) > 0 {
		fmt.Println("Recent messages:")
		if currentNodeID != "" {
			// Show path to current node
			printPathToNode(nodes, currentNodeID)
		} else {
			// Show last 4 nodes
			start := 0
			if len(nodes) > 4 {
				start = len(nodes) - 4
			}
			for _, node := range nodes[start:] {
				printNode(node)
			}
		}
		fmt.Println()
	}

	// Run interactive loop with current node tracking
	runInteractiveChatFromNode(ctx, mgr, dag, currentNodeID)
}

func runInteractiveChat(ctx context.Context, mgr *conversation.Manager, dag *types.DAG) {
	runInteractiveChatFromNode(ctx, mgr, dag, "")
}

func runInteractiveChatFromNode(ctx context.Context, mgr *conversation.Manager, dag *types.DAG, startNodeID string) {
	reader := bufio.NewReader(os.Stdin)
	titleSet := dag.Title != ""
	currentNodeID := startNodeID

	for {
		fmt.Print("You> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle commands
		if strings.HasPrefix(input, "/") {
			if handleChatCommand(ctx, mgr, dag, input) {
				continue
			}
			return // /quit or /exit
		}

		// Set title from first message if not set
		if !titleSet {
			title := mgr.GenerateTitle(input)
			mgr.UpdateTitle(ctx, dag.ID, title)
			titleSet = true
		}

		// Send message and stream response
		fmt.Print("\nAssistant> ")
		events, err := mgr.SendMessageAfter(ctx, dag.ID, currentNodeID, input)
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
			continue
		}

		for event := range events {
			switch event.Type {
			case types.StreamEventDelta:
				fmt.Print(event.Content)
			case types.StreamEventError:
				fmt.Printf("\nError: %v\n", event.Error)
			case types.StreamEventNodeSaved:
				// Update current position to the assistant's response node
				currentNodeID = event.NodeID
			}
		}
		fmt.Println()
	}
}

// printPathToNode prints the path from root to a specific node
func printPathToNode(nodes []*types.DAGNode, targetNodeID string) {
	// Build node map
	nodeMap := make(map[string]*types.DAGNode)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// Walk up from target to collect path
	var path []*types.DAGNode
	currentID := targetNodeID
	for currentID != "" {
		node, ok := nodeMap[currentID]
		if !ok {
			break
		}
		path = append(path, node)
		currentID = node.ParentID
	}

	// Reverse to get chronological order
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	// Print last 4 nodes of the path
	start := 0
	if len(path) > 4 {
		start = len(path) - 4
	}
	for _, node := range path[start:] {
		printNode(node)
	}
}

func handleChatCommand(ctx context.Context, mgr *conversation.Manager, dag *types.DAG, input string) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/quit", "/exit":
		fmt.Println("Goodbye!")
		return false
	case "/show":
		nodes, err := mgr.GetNodes(ctx, dag.ID)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return true
		}
		fmt.Printf("\nDAG: %s (%d nodes)\n", dag.ID, len(nodes))
		for i, node := range nodes {
			prefix := "├─"
			if i == len(nodes)-1 {
				prefix = "└─"
			}
			fmt.Printf("%s ", prefix)
			printNodeCompact(node)
		}
		fmt.Println()
		return true
	case "/help":
		fmt.Println("\nAvailable commands:")
		fmt.Println("  /show     - Show DAG history")
		fmt.Println("  /help     - Show this help")
		fmt.Println("  /quit     - Exit the conversation")
		fmt.Println()
		return true
	default:
		fmt.Printf("Unknown command: %s (type /help for available commands)\n", cmd)
		return true
	}
}

func printNode(node *types.DAGNode) {
	var content string
	if err := json.Unmarshal(node.Content, &content); err != nil {
		content = string(node.Content)
	}

	role := string(node.NodeType)
	if node.NodeType == types.NodeTypeAssistant {
		role = "assistant"
	}

	// Truncate long content
	if len(content) > 200 {
		content = content[:197] + "..."
	}

	fmt.Printf("[%s] %s\n", role, content)
}

func printNodeCompact(node *types.DAGNode) {
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

	if node.TokensIn > 0 || node.TokensOut > 0 {
		fmt.Printf("%s [%s]: %s (tokens: %d/%d)\n", node.ID[:8], role, content, node.TokensIn, node.TokensOut)
	} else {
		fmt.Printf("%s [%s]: %s\n", node.ID[:8], role, content)
	}
}
