package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Output format flags (set by persistent flags in root.go)
var (
	outputJSON bool
	outputYAML bool
)

// getOutputFormat returns the current output format based on flags.
func getOutputFormat() string {
	if outputJSON {
		return "json"
	}
	if outputYAML {
		return "yaml"
	}
	return "default"
}

// printJSON marshals v as JSON and prints it to stdout.
func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printYAML marshals v as YAML and prints it to stdout.
func printYAML(v interface{}) error {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	defer enc.Close()
	return enc.Encode(v)
}

// printFormatted prints v in JSON or YAML format based on flags.
// Returns true if output was printed, false if default format should be used.
func printFormatted(v interface{}) bool {
	if outputJSON {
		if err := printJSON(v); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		return true
	}
	if outputYAML {
		if err := printYAML(v); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding YAML: %v\n", err)
			os.Exit(1)
		}
		return true
	}
	return false
}
