// Package cli provides the command-line interface for langdag.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "langdag",
	Short: "LangDAG - LLM Conversation DAG Manager",
	Long: `LangDAG is a high-performance Go tool for managing LLM conversations as
directed acyclic graphs (DAGs).

It provides two modes:
  - Workflow mode: Pre-defined pipelines with static DAG structure
  - Chat mode: Interactive sessions that grow dynamically

Both modes create DAG instances that can be inspected and continued.

Examples:
  langdag prompt "What is LangDAG?"   # Start new conversation
  langdag prompt <node-id> "More"    # Continue from a node
  langdag ls                         # List all conversations
  langdag show <id>                  # Show node tree
  langdag rm <id>                    # Delete node + subtree
  langdag workflow create <file>     # Create workflow template
  langdag workflow ls                # List all workflows`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/langdag/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&outputYAML, "yaml", false, "output in YAML format")
	rootCmd.MarkFlagsMutuallyExclusive("json", "yaml")

	// Add subcommands
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(promptCmd)
	rootCmd.AddCommand(workflowCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
}

// versionCmd shows version information.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("langdag version 0.2.0")
	},
}

// exitError prints an error message and exits.
func exitError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+msg+"\n", args...)
	os.Exit(1)
}
