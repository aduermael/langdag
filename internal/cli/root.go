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
  langdag workflow create <file>     # Create workflow template from YAML
  langdag workflow list              # List all workflow templates
  langdag workflow run <name>        # Execute workflow → creates a DAG
  langdag dag list                   # List all DAG instances
  langdag dag show <id>              # Show DAG with its nodes
  langdag chat new                   # Start new chat → creates a DAG
  langdag chat continue <id>         # Continue any DAG interactively`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/langdag/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	// Add subcommands
	rootCmd.AddCommand(workflowCmd)
	rootCmd.AddCommand(dagCmd)
	rootCmd.AddCommand(chatCmd)
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
