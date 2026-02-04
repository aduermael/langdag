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
	Use:   "continue <dag-id>",
	Short: "Continue an existing DAG",
	Long:  `Continue an existing DAG by attaching a human node. Works with any DAG - from chat or workflow.`,
	Args:  cobra.ExactArgs(1),
	Run:   runChatContinue,
}

func init() {
	chatCmd.AddCommand(chatNewCmd)
	chatCmd.AddCommand(chatContinueCmd)

	chatNewCmd.Flags().StringVarP(&chatModel, "model", "m", "claude-sonnet-4-20250514", "model to use")
	chatNewCmd.Flags().StringVarP(&chatSystemPrompt, "system", "s", "", "system prompt")
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
	dagID := args[0]

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

	// Get DAG - try full ID first, then partial match
	dag, err := mgr.GetDAG(ctx, dagID)
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
	if dag.Title != "" {
		fmt.Printf("Title: %s\n", dag.Title)
	}
	if dag.WorkflowID != "" {
		fmt.Printf("From workflow: %s\n", dag.WorkflowID)
	}
	fmt.Println()

	// Show recent history
	nodes, err := mgr.GetNodes(ctx, dagID)
	if err == nil && len(nodes) > 0 {
		fmt.Println("Recent messages:")
		start := 0
		if len(nodes) > 4 {
			start = len(nodes) - 4
		}
		for _, node := range nodes[start:] {
			printNode(node)
		}
		fmt.Println()
	}

	// Run interactive loop
	runInteractiveChat(ctx, mgr, dag)
}

func runInteractiveChat(ctx context.Context, mgr *conversation.Manager, dag *types.DAG) {
	reader := bufio.NewReader(os.Stdin)
	titleSet := dag.Title != ""

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
		events, err := mgr.SendMessage(ctx, dag.ID, input)
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
			}
		}
		fmt.Println()
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
