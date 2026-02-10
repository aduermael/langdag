package cli

import (
	"bufio"
	"context"
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
	promptModel        string
	promptSystemPrompt string
)

// promptCmd handles prompting — new conversations or continuing from a node.
var promptCmd = &cobra.Command{
	Use:   "prompt [node-id] [message]",
	Short: "Send a prompt",
	Long: `Send a prompt to start a new conversation or continue from a node.

Examples:
  langdag prompt "What is LangDAG?"                  # new conversation
  langdag prompt <node-id> "Tell me more"            # continue from node
  langdag prompt                                     # interactive mode (new)
  langdag prompt <node-id>                           # interactive mode from node`,
	Run: runPrompt,
}

func init() {
	promptCmd.Flags().StringVarP(&promptModel, "model", "m", "claude-sonnet-4-20250514", "model to use")
	promptCmd.Flags().StringVarP(&promptSystemPrompt, "system", "s", "", "system prompt")
}

func runPrompt(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		exitError("failed to load config: %v", err)
	}

	apiKey := cfg.Providers.Anthropic.APIKey
	if apiKey == "" {
		exitError("ANTHROPIC_API_KEY not set")
	}

	store, err := initStorage(ctx, cfg)
	if err != nil {
		exitError("%v", err)
	}
	defer store.Close()

	prov := anthropic.New(apiKey)
	mgr := conversation.NewManager(store, prov)

	// Parse args: [node-id] [message]
	var nodeID, message string
	switch len(args) {
	case 0:
		// Interactive mode, new conversation
	case 1:
		// Could be a node-id or a message
		if isNodeID(ctx, store, args[0]) {
			nodeID = args[0]
		} else {
			message = args[0]
		}
	default:
		nodeID = args[0]
		message = strings.Join(args[1:], " ")
	}

	if nodeID != "" {
		// Resolve the node
		node, err := mgr.ResolveNode(ctx, nodeID)
		if err != nil {
			exitError("failed to resolve node: %v", err)
		}
		if node == nil {
			exitError("node not found: %s", nodeID)
		}

		if message != "" {
			// Single prompt from node
			sendAndPrint(ctx, mgr, node.ID, message)
		} else {
			// Interactive from node
			fmt.Printf("Continuing from node %s\n", node.ID[:8])
			if node.Title != "" {
				fmt.Printf("Title: %s\n", node.Title)
			}
			fmt.Println()
			runInteractive(ctx, mgr, node.ID)
		}
	} else {
		if message != "" {
			// Single prompt, new conversation
			sendAndPrintNew(ctx, mgr, message)
		} else {
			// Interactive, new conversation
			fmt.Println("Starting new conversation")
			if promptSystemPrompt != "" {
				fmt.Printf("System: %s\n", promptSystemPrompt)
			}
			fmt.Println()
			runInteractiveNew(ctx, mgr)
		}
	}
}

// sendAndPrintNew creates a new conversation and prints the response.
func sendAndPrintNew(ctx context.Context, mgr *conversation.Manager, message string) {
	events, err := mgr.Prompt(ctx, message, promptModel, promptSystemPrompt)
	if err != nil {
		exitError("prompt failed: %v", err)
	}
	for event := range events {
		switch event.Type {
		case types.StreamEventDelta:
			fmt.Print(event.Content)
		case types.StreamEventError:
			fmt.Printf("\nError: %v\n", event.Error)
			return
		case types.StreamEventNodeSaved:
			fmt.Printf("\n\n(node: %s)\n", event.NodeID[:8])
		}
	}
}

// sendAndPrint continues from a node and prints the response.
func sendAndPrint(ctx context.Context, mgr *conversation.Manager, parentNodeID, message string) {
	events, err := mgr.PromptFrom(ctx, parentNodeID, message, "")
	if err != nil {
		exitError("prompt failed: %v", err)
	}
	for event := range events {
		switch event.Type {
		case types.StreamEventDelta:
			fmt.Print(event.Content)
		case types.StreamEventError:
			fmt.Printf("\nError: %v\n", event.Error)
			return
		case types.StreamEventNodeSaved:
			fmt.Printf("\n\n(node: %s)\n", event.NodeID[:8])
		}
	}
}

// runInteractiveNew runs interactive mode for a new conversation.
func runInteractiveNew(ctx context.Context, mgr *conversation.Manager) {
	reader := bufio.NewReader(os.Stdin)
	var currentNodeID string

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
		if input == "/quit" || input == "/exit" {
			fmt.Println("Goodbye!")
			return
		}
		if input == "/help" {
			fmt.Println("\nCommands: /quit, /help")
			fmt.Println()
			continue
		}

		fmt.Print("\nAssistant> ")
		if currentNodeID == "" {
			events, err := mgr.Prompt(ctx, input, promptModel, promptSystemPrompt)
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
					currentNodeID = event.NodeID
				}
			}
		} else {
			events, err := mgr.PromptFrom(ctx, currentNodeID, input, "")
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
					currentNodeID = event.NodeID
				}
			}
		}
		fmt.Println()
	}
}

// runInteractive runs interactive mode from an existing node.
func runInteractive(ctx context.Context, mgr *conversation.Manager, startNodeID string) {
	reader := bufio.NewReader(os.Stdin)
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
		if input == "/quit" || input == "/exit" {
			fmt.Println("Goodbye!")
			return
		}
		if input == "/help" {
			fmt.Println("\nCommands: /quit, /help")
			fmt.Println()
			continue
		}

		fmt.Print("\nAssistant> ")
		events, err := mgr.PromptFrom(ctx, currentNodeID, input, "")
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
				currentNodeID = event.NodeID
			}
		}
		fmt.Println()
	}
}

// isNodeID checks if a string looks like it could be a node ID.
func isNodeID(ctx context.Context, store *sqlite.SQLiteStorage, s string) bool {
	// If it's at least 4 chars and resolves to a node, it's a node ID
	if len(s) < 4 {
		return false
	}
	node, _ := store.GetNode(ctx, s)
	if node != nil {
		return true
	}
	node, _ = store.GetNodeByPrefix(ctx, s)
	return node != nil
}
