package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/langdag/langdag/internal/api"
	"github.com/langdag/langdag/internal/config"
	"github.com/spf13/cobra"
)

var (
	servePort   int
	serveHost   string
	serveAPIKey string
)

// serveCmd starts the API server.
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the API server",
	Long: `Start the LangDAG API server.

The server provides REST endpoints for:
  - DAG management (list, get, delete)
  - Chat (new, continue, fork) with SSE streaming
  - Workflow management and execution

Example:
  langdag serve --port 8080
  langdag serve --host 0.0.0.0 --port 3000 --api-key secret`,
	Run: runServe,
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "port to listen on")
	serveCmd.Flags().StringVarP(&serveHost, "host", "H", "127.0.0.1", "host to bind to")
	serveCmd.Flags().StringVar(&serveAPIKey, "api-key", "", "API key for authentication (optional)")

	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		exitError("failed to load config: %v", err)
	}

	// Create server
	addr := fmt.Sprintf("%s:%d", serveHost, servePort)
	serverCfg := &api.Config{
		Addr:   addr,
		APIKey: serveAPIKey,
	}

	server, err := api.New(serverCfg, cfg)
	if err != nil {
		exitError("failed to create server: %v", err)
	}

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		fmt.Println("\nShutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
		}
	}()

	// Print startup message
	fmt.Printf("LangDAG API server starting on http://%s\n", addr)
	fmt.Println()
	fmt.Println("Endpoints:")
	fmt.Println("  GET  /health            - Health check")
	fmt.Println("  GET  /dags              - List all DAGs")
	fmt.Println("  GET  /dags/{id}         - Get DAG details")
	fmt.Println("  DELETE /dags/{id}       - Delete a DAG")
	fmt.Println("  POST /chat              - Start new conversation")
	fmt.Println("  POST /chat/{id}         - Continue conversation")
	fmt.Println("  POST /chat/{id}/fork    - Fork from a node")
	fmt.Println("  GET  /workflows         - List workflows")
	fmt.Println("  POST /workflows         - Create workflow")
	fmt.Println("  POST /workflows/{id}/run - Run workflow")
	fmt.Println()
	if serveAPIKey != "" {
		fmt.Println("Authentication: Required (use Authorization: Bearer <key> or X-API-Key header)")
	} else {
		fmt.Println("Authentication: Disabled (use --api-key to enable)")
	}
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop")

	// Start server
	if err := server.Start(); err != nil && err.Error() != "http: Server closed" {
		exitError("server error: %v", err)
	}
}
