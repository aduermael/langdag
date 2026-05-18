package main

import (
	"flag"
	"fmt"
	"os"

	"langdag.com/langdag/internal/models"
)

func main() {
	left := flag.String("left", "", "path to the existing catalog")
	right := flag.String("right", "", "path to the candidate catalog")
	flag.Parse()

	if *left == "" || *right == "" {
		fmt.Fprintln(os.Stderr, "catalogcmp: -left and -right are required")
		os.Exit(2)
	}

	equal, err := models.CatalogV1SemanticEqual(*left, *right)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalogcmp: %v\n", err)
		os.Exit(2)
	}
	if equal {
		os.Exit(0)
	}
	os.Exit(1)
}
