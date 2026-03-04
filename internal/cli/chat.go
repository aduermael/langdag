package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/langdag/langdag/internal/config"
	"github.com/langdag/langdag/pkg/langdag"
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

	client, err := newLibraryClient(ctx)
	if err != nil {
		exitError("%v", err)
	}
	defer client.Close()

	// Parse args: [node-id] [message]
	var nodeID, message string
	switch len(args) {
	case 0:
		// Interactive mode, new conversation
	case 1:
		// Could be a node-id or a message — try to resolve as node first
		node, _ := client.GetNode(ctx, args[0])
		if node != nil {
			nodeID = node.ID
		} else {
			message = args[0]
		}
	default:
		// First arg treated as node-id, rest as message
		node, _ := client.GetNode(ctx, args[0])
		if node != nil {
			nodeID = node.ID
			message = strings.Join(args[1:], " ")
		} else {
			message = strings.Join(args, " ")
		}
	}

	promptOpts := []langdag.PromptOption{
		langdag.WithModel(promptModel),
	}
	if promptSystemPrompt != "" {
		promptOpts = append(promptOpts, langdag.WithSystemPrompt(promptSystemPrompt))
	}

	if nodeID != "" {
		if message != "" {
			// Single prompt from node
			sendAndPrint(ctx, client, nodeID, message, promptOpts...)
		} else {
			// Interactive from node
			fmt.Printf("Continuing from node %s\n", nodeID[:8])
			fmt.Println()
			runInteractive(ctx, client, nodeID, promptOpts...)
		}
	} else {
		if message != "" {
			// Single prompt, new conversation
			sendAndPrintNew(ctx, client, message, promptOpts...)
		} else {
			// Interactive, new conversation
			fmt.Println("Starting new conversation")
			if promptSystemPrompt != "" {
				fmt.Printf("System: %s\n", promptSystemPrompt)
			}
			fmt.Println()
			runInteractiveNew(ctx, client, promptOpts...)
		}
	}
}

// newLibraryClient creates a langdag.Client from the loaded config.
func newLibraryClient(ctx context.Context) (*langdag.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	storagePath := cfg.Storage.Path
	if storagePath == "./langdag.db" {
		storagePath = config.GetDefaultStoragePath()
	}

	libCfg := langdag.Config{
		StoragePath: storagePath,
		Provider:    cfg.Providers.Default,
		APIKeys: map[string]string{
			"anthropic": cfg.Providers.Anthropic.APIKey,
			"openai":    cfg.Providers.OpenAI.APIKey,
			"gemini":    cfg.Providers.Gemini.APIKey,
		},
	}

	if cfg.Providers.OpenAI.BaseURL != "" {
		libCfg.OpenAIConfig = &langdag.OpenAIConfig{BaseURL: cfg.Providers.OpenAI.BaseURL}
	}

	if cfg.Providers.AnthropicVertex.ProjectID != "" {
		libCfg.VertexConfig = &langdag.VertexConfig{
			ProjectID: cfg.Providers.AnthropicVertex.ProjectID,
			Region:    cfg.Providers.AnthropicVertex.Region,
		}
	} else if cfg.Providers.GeminiVertex.ProjectID != "" {
		libCfg.VertexConfig = &langdag.VertexConfig{
			ProjectID: cfg.Providers.GeminiVertex.ProjectID,
			Region:    cfg.Providers.GeminiVertex.Region,
		}
	}

	if cfg.Providers.OpenAIAzure.APIKey != "" {
		libCfg.AzureOpenAIConfig = &langdag.AzureOpenAIConfig{
			APIKey:     cfg.Providers.OpenAIAzure.APIKey,
			Endpoint:   cfg.Providers.OpenAIAzure.Endpoint,
			APIVersion: cfg.Providers.OpenAIAzure.APIVersion,
		}
	}

	if cfg.Retry.MaxRetries > 0 || cfg.Retry.BaseDelay != "" || cfg.Retry.MaxDelay != "" {
		rc := &langdag.RetryConfig{}
		if cfg.Retry.MaxRetries > 0 {
			rc.MaxRetries = cfg.Retry.MaxRetries
		}
		libCfg.RetryConfig = rc
	}

	// Map routing entries
	for _, re := range cfg.Providers.Routing {
		entry := langdag.RoutingEntry{
			Provider: re.Provider,
			Weight:   re.Weight,
		}
		libCfg.Routing = append(libCfg.Routing, entry)
	}
	libCfg.FallbackOrder = cfg.Providers.FallbackOrder

	return langdag.New(libCfg)
}

// sendAndPrintNew creates a new conversation and prints the response.
func sendAndPrintNew(ctx context.Context, client *langdag.Client, message string, opts ...langdag.PromptOption) {
	result, err := client.Prompt(ctx, message, opts...)
	if err != nil {
		exitError("prompt failed: %v", err)
	}
	for chunk := range result.Stream {
		if chunk.Error != nil {
			fmt.Printf("\nError: %v\n", chunk.Error)
			return
		}
		if chunk.Done {
			fmt.Printf("\n\n(node: %s)\n", chunk.NodeID[:8])
		} else {
			fmt.Print(chunk.Content)
		}
	}
}

// sendAndPrint continues from a node and prints the response.
func sendAndPrint(ctx context.Context, client *langdag.Client, parentNodeID, message string, opts ...langdag.PromptOption) {
	result, err := client.PromptFrom(ctx, parentNodeID, message, opts...)
	if err != nil {
		exitError("prompt failed: %v", err)
	}
	for chunk := range result.Stream {
		if chunk.Error != nil {
			fmt.Printf("\nError: %v\n", chunk.Error)
			return
		}
		if chunk.Done {
			fmt.Printf("\n\n(node: %s)\n", chunk.NodeID[:8])
		} else {
			fmt.Print(chunk.Content)
		}
	}
}

// runInteractiveNew runs interactive mode for a new conversation.
func runInteractiveNew(ctx context.Context, client *langdag.Client, opts ...langdag.PromptOption) {
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
		var result *langdag.PromptResult
		if currentNodeID == "" {
			result, err = client.Prompt(ctx, input, opts...)
		} else {
			result, err = client.PromptFrom(ctx, currentNodeID, input, opts...)
		}
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
			continue
		}
		for chunk := range result.Stream {
			if chunk.Error != nil {
				fmt.Printf("\nError: %v\n", chunk.Error)
				break
			}
			if chunk.Done {
				currentNodeID = chunk.NodeID
			} else {
				fmt.Print(chunk.Content)
			}
		}
		fmt.Println()
	}
}

// runInteractive runs interactive mode from an existing node.
func runInteractive(ctx context.Context, client *langdag.Client, startNodeID string, opts ...langdag.PromptOption) {
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
		result, err := client.PromptFrom(ctx, currentNodeID, input, opts...)
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
			continue
		}
		for chunk := range result.Stream {
			if chunk.Error != nil {
				fmt.Printf("\nError: %v\n", chunk.Error)
				break
			}
			if chunk.Done {
				currentNodeID = chunk.NodeID
			} else {
				fmt.Print(chunk.Content)
			}
		}
		fmt.Println()
	}
}
