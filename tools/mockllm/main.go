// Package main implements a mock LLM server that mimics the Anthropic API.
// Used for testing SDKs and the LangDAG server without consuming AI tokens.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := parseFlags()

	server := NewServer(cfg)

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		fmt.Println("\nShutting down mock LLM server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.httpServer.Shutdown(ctx)
	}()

	addr := fmt.Sprintf(":%d", cfg.Port)
	fmt.Printf("Mock LLM server starting on http://localhost%s\n", addr)
	fmt.Printf("Mode: %s | Delay: %s | Chunk delay: %s\n", cfg.Mode, cfg.Delay, cfg.ChunkDelay)

	if err := server.Start(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func parseFlags() *Config {
	cfg := &Config{}
	flag.IntVar(&cfg.Port, "port", 9090, "port to listen on")
	flag.StringVar(&cfg.Mode, "mode", "random", "response mode: random, echo, fixed, error")
	flag.DurationVar(&cfg.Delay, "delay", 0, "delay before responding")
	flag.DurationVar(&cfg.ChunkDelay, "chunk-delay", 50*time.Millisecond, "delay between SSE chunks")
	flag.IntVar(&cfg.ChunkSize, "chunk-size", 3, "number of words per SSE chunk")
	flag.StringVar(&cfg.FixedResponse, "response", "", "fixed response text (for mode=fixed)")
	flag.IntVar(&cfg.ErrorCode, "error-code", 500, "HTTP error code (for mode=error)")
	flag.StringVar(&cfg.ErrorMessage, "error-message", "internal server error", "error message (for mode=error)")
	flag.Parse()
	return cfg
}
