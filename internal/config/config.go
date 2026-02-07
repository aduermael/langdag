// Package config handles configuration loading for langdag.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the application configuration.
type Config struct {
	Storage   StorageConfig   `mapstructure:"storage"`
	Providers ProvidersConfig `mapstructure:"providers"`
	Server    ServerConfig    `mapstructure:"server"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Execution ExecutionConfig `mapstructure:"execution"`
}

// StorageConfig represents storage configuration.
type StorageConfig struct {
	Driver     string `mapstructure:"driver"`
	Path       string `mapstructure:"path"`
	Connection string `mapstructure:"connection"`
}

// ProvidersConfig represents provider configurations.
type ProvidersConfig struct {
	Default   string             `mapstructure:"default"`
	Anthropic ProviderConfig     `mapstructure:"anthropic"`
	OpenAI    ProviderConfig     `mapstructure:"openai"`
	Mock      MockProviderConfig `mapstructure:"mock"`
}

// ProviderConfig represents a single provider configuration.
type ProviderConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
}

// MockProviderConfig represents mock provider configuration.
type MockProviderConfig struct {
	Mode          string `mapstructure:"mode"`           // random, echo, fixed
	FixedResponse string `mapstructure:"fixed_response"` // response for fixed mode
	Delay         string `mapstructure:"delay"`           // delay before responding
	ChunkDelay    string `mapstructure:"chunk_delay"`     // delay between stream chunks
}

// ServerConfig represents server configuration.
type ServerConfig struct {
	Host        string   `mapstructure:"host"`
	Port        int      `mapstructure:"port"`
	CORSOrigins []string `mapstructure:"cors_origins"`
}

// LoggingConfig represents logging configuration.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// ExecutionConfig represents execution configuration.
type ExecutionConfig struct {
	DefaultTimeout string `mapstructure:"default_timeout"`
	MaxParallel    int    `mapstructure:"max_parallel"`
	RetryAttempts  int    `mapstructure:"retry_attempts"`
}

// Load loads the configuration from files and environment variables.
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Set config name and paths
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// Add config paths
	v.AddConfigPath(".")
	v.AddConfigPath("./langdag")

	// Add user config directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		v.AddConfigPath(filepath.Join(homeDir, ".config", "langdag"))
	}

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK, we'll use defaults and env vars
	}

	// Bind environment variables
	v.SetEnvPrefix("LANGDAG")
	v.AutomaticEnv()

	// Also support direct env var names
	v.BindEnv("providers.default", "LANGDAG_PROVIDER")
	v.BindEnv("providers.anthropic.api_key", "ANTHROPIC_API_KEY")
	v.BindEnv("providers.openai.api_key", "OPENAI_API_KEY")
	v.BindEnv("providers.mock.mode", "LANGDAG_MOCK_MODE")
	v.BindEnv("providers.mock.fixed_response", "LANGDAG_MOCK_RESPONSE")
	v.BindEnv("providers.mock.delay", "LANGDAG_MOCK_DELAY")
	v.BindEnv("providers.mock.chunk_delay", "LANGDAG_MOCK_CHUNK_DELAY")
	v.BindEnv("storage.path", "LANGDAG_STORAGE_PATH")

	// Unmarshal config
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Expand environment variables in paths
	cfg.Storage.Path = os.ExpandEnv(cfg.Storage.Path)

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	// Storage defaults
	v.SetDefault("storage.driver", "sqlite")
	v.SetDefault("storage.path", "./langdag.db")

	// Provider defaults
	v.SetDefault("providers.default", "anthropic")
	v.SetDefault("providers.mock.mode", "random")

	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.cors_origins", []string{"*"})

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")

	// Execution defaults
	v.SetDefault("execution.default_timeout", "300s")
	v.SetDefault("execution.max_parallel", 10)
	v.SetDefault("execution.retry_attempts", 3)
}

// GetDefaultStoragePath returns the default storage path.
func GetDefaultStoragePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./langdag.db"
	}
	return filepath.Join(homeDir, ".config", "langdag", "langdag.db")
}

// EnsureConfigDir ensures the config directory exists.
func EnsureConfigDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(homeDir, ".config", "langdag")
	return os.MkdirAll(configDir, 0755)
}

// EnsureStorageDir ensures the directory for the storage path exists.
func EnsureStorageDir(storagePath string) error {
	dir := filepath.Dir(storagePath)
	return os.MkdirAll(dir, 0755)
}
