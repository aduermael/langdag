package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/langdag/langdag/internal/config"
	"github.com/langdag/langdag/internal/executor"
	"github.com/langdag/langdag/internal/provider/anthropic"
	"github.com/langdag/langdag/internal/workflow"
	"github.com/langdag/langdag/pkg/types"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	workflowInputFile string
	workflowStream    bool
)

// workflowCmd is the parent command for workflow operations.
var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage workflow templates",
	Long:  `Commands for managing workflow templates - pre-defined DAG pipelines.`,
}

// workflowCreateCmd creates a new workflow.
var workflowCreateCmd = &cobra.Command{
	Use:   "create <file>",
	Short: "Create a workflow from a YAML file",
	Long:  `Create a new workflow template from a YAML definition file.`,
	Args:  cobra.ExactArgs(1),
	Run:   runWorkflowCreate,
}

// workflowListCmd lists all workflows.
var workflowListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List all workflows",
	Long:    `List all stored workflow templates.`,
	Run:     runWorkflowList,
}

// workflowShowCmd shows a workflow.
var workflowShowCmd = &cobra.Command{
	Use:   "show <id-or-name>",
	Short: "Show workflow definition",
	Long:  `Show the definition of a workflow template.`,
	Args:  cobra.ExactArgs(1),
	Run:   runWorkflowShow,
}

// workflowValidateCmd validates a workflow file.
var workflowValidateCmd = &cobra.Command{
	Use:   "validate <file>",
	Short: "Validate a workflow file",
	Long:  `Validate a workflow definition file without creating it.`,
	Args:  cobra.ExactArgs(1),
	Run:   runWorkflowValidate,
}

// workflowDeleteCmd deletes a workflow.
var workflowDeleteCmd = &cobra.Command{
	Use:     "rm <id-or-name>",
	Aliases: []string{"delete"},
	Short:   "Delete a workflow",
	Long:    `Delete a workflow template.`,
	Args:    cobra.ExactArgs(1),
	Run:     runWorkflowDelete,
}

// workflowRunCmd runs a workflow.
var workflowRunCmd = &cobra.Command{
	Use:   "run <id-or-name>",
	Short: "Run a workflow",
	Long:  `Execute a workflow template, creating a new DAG instance.`,
	Args:  cobra.ExactArgs(1),
	Run:   runWorkflowRun,
}

func init() {
	workflowCmd.AddCommand(workflowCreateCmd)
	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
	workflowCmd.AddCommand(workflowDeleteCmd)
	workflowCmd.AddCommand(workflowRunCmd)

	workflowRunCmd.Flags().StringVarP(&workflowInputFile, "input", "i", "", "input JSON file or inline JSON")
	workflowRunCmd.Flags().BoolVar(&workflowStream, "stream", true, "stream output")
}

func runWorkflowCreate(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	filePath := args[0]

	// Parse the workflow file
	wf, err := workflow.ParseFile(filePath)
	if err != nil {
		exitError("failed to parse workflow file: %v", err)
	}

	// Validate
	result := workflow.Validate(wf)
	if !result.Valid {
		exitError("workflow validation failed:\n%s", result.FormatErrors())
	}

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

	// Create workflow manager
	mgr := workflow.NewManager(store)

	// Check if workflow with same name exists
	existing, _ := mgr.GetByName(ctx, wf.Name)
	if existing != nil {
		exitError("workflow with name '%s' already exists (id: %s)", wf.Name, existing.ID)
	}

	// Create workflow
	if err := mgr.Create(ctx, wf); err != nil {
		exitError("failed to create workflow: %v", err)
	}

	fmt.Printf("Created workflow: %s (id: %s)\n", wf.Name, wf.ID)
}

func runWorkflowList(cmd *cobra.Command, args []string) {
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

	// List workflows
	workflows, err := store.ListWorkflows(ctx)
	if err != nil {
		exitError("failed to list workflows: %v", err)
	}

	if len(workflows) == 0 {
		if outputJSON {
			fmt.Println("[]")
		} else if outputYAML {
			fmt.Println("[]")
		} else {
			fmt.Println("No workflows found.")
		}
		return
	}

	// Handle JSON/YAML output
	if printFormatted(workflows) {
		return
	}

	// Default: table output
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "Version", "Nodes", "Edges", "Created"})
	table.SetBorder(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	for _, wf := range workflows {
		name := wf.Name
		if len(name) > 20 {
			name = name[:17] + "..."
		}

		table.Append([]string{
			wf.ID[:8],
			name,
			fmt.Sprintf("%d", wf.Version),
			fmt.Sprintf("%d", len(wf.Nodes)),
			fmt.Sprintf("%d", len(wf.Edges)),
			wf.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	table.Render()
}

func runWorkflowShow(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	idOrName := args[0]

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

	// Get workflow
	mgr := workflow.NewManager(store)
	wf, err := mgr.Get(ctx, idOrName)
	if err != nil {
		exitError("failed to get workflow: %v", err)
	}
	if wf == nil {
		// Try by name
		wf, err = mgr.GetByName(ctx, idOrName)
		if err != nil {
			exitError("failed to get workflow: %v", err)
		}
	}
	if wf == nil {
		exitError("workflow not found: %s", idOrName)
	}

	// Handle JSON/YAML output
	if printFormatted(wf) {
		return
	}

	// Default: structured text output
	fmt.Printf("Workflow: %s (v%d)\n", wf.Name, wf.Version)
	fmt.Printf("ID: %s\n", wf.ID)
	if wf.Description != "" {
		fmt.Printf("Description: %s\n", wf.Description)
	}
	fmt.Printf("Created: %s\n", wf.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Print defaults
	if wf.Defaults.Model != "" || wf.Defaults.Provider != "" {
		fmt.Println("Defaults:")
		if wf.Defaults.Provider != "" {
			fmt.Printf("  Provider: %s\n", wf.Defaults.Provider)
		}
		if wf.Defaults.Model != "" {
			fmt.Printf("  Model: %s\n", wf.Defaults.Model)
		}
		if wf.Defaults.MaxTokens > 0 {
			fmt.Printf("  Max Tokens: %d\n", wf.Defaults.MaxTokens)
		}
		fmt.Println()
	}

	// Print tools
	if len(wf.Tools) > 0 {
		fmt.Printf("Tools (%d):\n", len(wf.Tools))
		for _, tool := range wf.Tools {
			fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
		}
		fmt.Println()
	}

	// Print nodes as tree
	fmt.Printf("Nodes (%d):\n", len(wf.Nodes))
	for _, node := range wf.Nodes {
		fmt.Printf("  %s [%s]", node.ID, node.Type)
		if node.Model != "" {
			fmt.Printf(" model=%s", node.Model)
		}
		if len(node.Tools) > 0 {
			fmt.Printf(" tools=%v", node.Tools)
		}
		fmt.Println()
	}
	fmt.Println()

	// Print edges
	fmt.Printf("Edges (%d):\n", len(wf.Edges))
	for _, edge := range wf.Edges {
		fmt.Printf("  %s -> %s", edge.From, edge.To)
		if edge.Condition != "" {
			fmt.Printf(" [condition: %s]", edge.Condition)
		}
		fmt.Println()
	}
}

func runWorkflowValidate(cmd *cobra.Command, args []string) {
	filePath := args[0]

	// Check file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		exitError("file not found: %s", filePath)
	}

	// Validate the workflow file
	result := workflow.ValidateFile(filePath)

	if result.Valid {
		fmt.Printf("Workflow file is valid: %s\n", filePath)
	} else {
		fmt.Printf("Workflow file is invalid: %s\n\n", filePath)
		fmt.Print(result.FormatErrors())
		os.Exit(1)
	}
}

func runWorkflowDelete(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	idOrName := args[0]

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

	// Get workflow to find ID
	mgr := workflow.NewManager(store)
	wf, _ := mgr.Get(ctx, idOrName)
	if wf == nil {
		wf, _ = mgr.GetByName(ctx, idOrName)
	}
	if wf == nil {
		exitError("workflow not found: %s", idOrName)
	}

	// Delete workflow
	if err := mgr.Delete(ctx, wf.ID); err != nil {
		exitError("failed to delete workflow: %v", err)
	}

	fmt.Printf("Deleted workflow: %s (id: %s)\n", wf.Name, wf.ID)
}

func runWorkflowRun(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	idOrName := args[0]

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

	// Get workflow
	workflowMgr := workflow.NewManager(store)
	wf, _ := workflowMgr.Get(ctx, idOrName)
	if wf == nil {
		wf, _ = workflowMgr.GetByName(ctx, idOrName)
	}
	if wf == nil {
		exitError("workflow not found: %s", idOrName)
	}

	// Parse input
	var input json.RawMessage
	if workflowInputFile != "" {
		// Check if it's a file or inline JSON
		if _, err := os.Stat(workflowInputFile); err == nil {
			data, err := os.ReadFile(workflowInputFile)
			if err != nil {
				exitError("failed to read input file: %v", err)
			}
			input = data
		} else {
			input = json.RawMessage(workflowInputFile)
		}
	} else {
		input = json.RawMessage(`{}`)
	}

	// Validate input is valid JSON
	var check interface{}
	if err := json.Unmarshal(input, &check); err != nil {
		exitError("invalid input JSON: %v", err)
	}

	// Create provider
	prov := anthropic.New(apiKey)

	// Create executor
	exec := executor.NewExecutor(store, prov)

	fmt.Printf("Running workflow: %s\n", wf.Name)

	// Execute workflow
	events, err := exec.Execute(ctx, wf, input, executor.ExecuteOptions{Stream: workflowStream})
	if err != nil {
		exitError("failed to execute workflow: %v", err)
	}

	// Process events
	for event := range events {
		switch event.Type {
		case "node_start":
			fmt.Printf("├─ %s: starting...\n", event.NodeID)
		case "stream":
			fmt.Print(event.Content)
		case "node_complete":
			if event.Output != nil {
				var output string
				if err := json.Unmarshal(event.Output, &output); err == nil {
					if len(output) > 100 {
						output = output[:97] + "..."
					}
					fmt.Printf("├─ %s: %s\n", event.NodeID, output)
				}
			}
		case "error":
			fmt.Printf("└─ Error: %s\n", event.Error)
		case "done":
			fmt.Println("└─ Complete")
			if event.Output != nil {
				fmt.Printf("\nResult: %s\n", string(event.Output))
			}
			fmt.Printf("Root Node ID: %s\n", event.RootNodeID)
		}
	}
}

func printWorkflowTree(wf *types.Workflow) {
	// Get topological order
	order, err := workflow.TopologicalSort(wf)
	if err != nil {
		fmt.Println("Error: could not determine node order")
		return
	}

	fmt.Printf("\n%s\n│\n", wf.Name)

	for i, nodeID := range order {
		node := workflow.GetNode(wf, nodeID)
		if node == nil {
			continue
		}

		prefix := "├─►"
		if i == len(order)-1 {
			prefix = "└─►"
		}

		fmt.Printf("%s %s [%s]\n", prefix, node.ID, node.Type)

		// Print node details
		if node.Model != "" {
			fmt.Printf("│   model: %s\n", node.Model)
		}
		if len(node.Tools) > 0 {
			fmt.Printf("│   tools: %v\n", node.Tools)
		}
	}
}
