package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/langdag/langdag/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// configCmd is the parent command for config operations.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `Commands for managing LangDAG configuration.`,
}

// configShowCmd shows the current configuration.
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current configuration values.`,
	Run:   runConfigShow,
}

// configPathCmd shows the config file path.
var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show config file path",
	Long:  `Display the path to the configuration file.`,
	Run:   runConfigPath,
}

// configInitCmd initializes a config file.
var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration file",
	Long:  `Create a default configuration file.`,
	Run:   runConfigInit,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configInitCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) {
	cfg, err := config.Load()
	if err != nil {
		exitError("failed to load config: %v", err)
	}

	// Convert to YAML for display
	data, err := yaml.Marshal(cfg)
	if err != nil {
		exitError("failed to marshal config: %v", err)
	}

	fmt.Println("Current configuration:")
	fmt.Println()
	fmt.Println(string(data))
}

func runConfigPath(cmd *cobra.Command, args []string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		exitError("failed to get home directory: %v", err)
	}

	configPath := filepath.Join(homeDir, ".config", "langdag", "config.yaml")
	fmt.Printf("Config file path: %s\n", configPath)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("(file does not exist)")
	} else {
		fmt.Println("(file exists)")
	}
}

func runConfigInit(cmd *cobra.Command, args []string) {
	if err := config.EnsureConfigDir(); err != nil {
		exitError("failed to create config directory: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		exitError("failed to get home directory: %v", err)
	}

	configPath := filepath.Join(homeDir, ".config", "langdag", "config.yaml")

	// Check if file exists
	if _, err := os.Stat(configPath); err == nil {
		exitError("config file already exists: %s", configPath)
	}

	// Create default config
	defaultConfig := `# LangDAG Configuration

# Storage settings
storage:
  driver: sqlite
  path: ~/.config/langdag/langdag.db

# Provider settings
providers:
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
    # base_url: https://api.anthropic.com

# Logging settings
logging:
  level: info
  format: text

# Execution settings
execution:
  default_timeout: 300s
  max_parallel: 10
  retry_attempts: 3
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		exitError("failed to write config file: %v", err)
	}

	fmt.Printf("Created config file: %s\n", configPath)
	fmt.Println("\nNote: Set ANTHROPIC_API_KEY environment variable to use the Anthropic provider.")
}
