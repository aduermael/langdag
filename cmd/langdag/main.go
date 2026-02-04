// Package main provides the entry point for the langdag CLI.
package main

import (
	"fmt"
	"os"

	"github.com/langdag/langdag/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
