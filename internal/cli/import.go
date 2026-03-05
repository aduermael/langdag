package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/langdag/langdag/internal/migrate/langgraph"
	"github.com/langdag/langdag/internal/storage"
	"github.com/langdag/langdag/internal/storage/sqlite"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import data from other sources",
}

var importLangGraphCmd = &cobra.Command{
	Use:   "langgraph",
	Short: "Import conversations from a LangGraph database",
	Long: `Import conversation data from LangGraph into langdag.

Supports importing from:
  - JSON export files (use the langgraph-export Python tool to generate)
  - LangGraph SQLite databases (direct read)

Examples:
  # Import from JSON export file
  langdag import langgraph --file export.json

  # Import directly from LangGraph SQLite database
  langdag import langgraph --sqlite /path/to/langgraph.db

  # Preview what would be imported (dry run)
  langdag import langgraph --file export.json --dry-run

  # Import into a specific langdag database
  langdag import langgraph --file export.json --output /path/to/langdag.db`,
	RunE: runImportLangGraph,
}

var (
	importFile         string
	importSQLite       string
	importOutput       string
	importDryRun       bool
	importSkipExisting bool
)

func init() {
	importLangGraphCmd.Flags().StringVar(&importFile, "file", "", "Path to JSON export file")
	importLangGraphCmd.Flags().StringVar(&importSQLite, "sqlite", "", "Path to LangGraph SQLite database")
	importLangGraphCmd.Flags().StringVar(&importOutput, "output", "", "Path to langdag SQLite database (default: configured storage)")
	importLangGraphCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Preview import without writing")
	importLangGraphCmd.Flags().BoolVar(&importSkipExisting, "skip-existing", false, "Skip threads already in target database")

	importCmd.AddCommand(importLangGraphCmd)
	rootCmd.AddCommand(importCmd)
}

func runImportLangGraph(cmd *cobra.Command, args []string) error {
	// Exactly one of --file or --sqlite must be provided.
	if importFile == "" && importSQLite == "" {
		return fmt.Errorf("exactly one of --file or --sqlite must be specified")
	}
	if importFile != "" && importSQLite != "" {
		return fmt.Errorf("exactly one of --file or --sqlite must be specified")
	}

	ctx := context.Background()

	// Set up the target storage.
	var store storage.Storage
	var closeStore func()

	if importOutput != "" {
		// Create/open a SQLite database at the specified path.
		s, err := sqlite.New(importOutput)
		if err != nil {
			return fmt.Errorf("failed to open output database: %w", err)
		}
		if err := s.Init(ctx); err != nil {
			s.Close()
			return fmt.Errorf("failed to initialize output database: %w", err)
		}
		store = s
		closeStore = func() { s.Close() }
	} else {
		// Use the configured storage via the library client.
		client, err := newLibraryClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		store = client.Storage()
		closeStore = func() { client.Close() }
	}
	defer closeStore()

	// Load the export data from the specified source.
	var data *langgraph.ExportData

	if importFile != "" {
		if _, err := os.Stat(importFile); err != nil {
			return fmt.Errorf("cannot access file %q: %w", importFile, err)
		}
		d, err := readLangGraphExportFile(importFile)
		if err != nil {
			return err
		}
		data = d
	} else {
		if _, err := os.Stat(importSQLite); err != nil {
			return fmt.Errorf("cannot access SQLite database %q: %w", importSQLite, err)
		}
		reader, err := langgraph.NewSQLiteReader(importSQLite)
		if err != nil {
			return fmt.Errorf("failed to open LangGraph database: %w", err)
		}
		defer reader.Close()
		d, err := reader.ReadExportData(ctx)
		if err != nil {
			return fmt.Errorf("failed to read LangGraph database: %w", err)
		}
		data = d
	}

	total := len(data.Threads)

	// Dry run: just print what would be imported.
	if importDryRun {
		fmt.Fprintln(os.Stdout, "Dry run - no data will be written")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "Would import %d threads:\n", total)
		totalMessages := 0
		for _, thread := range data.Threads {
			fmt.Fprintf(os.Stdout, "  %s (%d messages)\n", thread.ThreadID, len(thread.Messages))
			totalMessages += len(thread.Messages)
		}
		printImportSummary(&langgraph.Result{
			ThreadsImported:  total,
			MessagesImported: totalMessages,
		})
		return nil
	}

	fmt.Fprintln(os.Stdout, "Importing LangGraph data...")

	opts := langgraph.ImportOptions{
		SkipExisting: importSkipExisting,
		Progress:     makeProgressFunc(total),
	}

	result, err := langgraph.ImportExportData(ctx, data, store, opts)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	printImportSummary(result)
	return nil
}

// makeProgressFunc returns a Progress callback that prints per-thread progress.
func makeProgressFunc(total int) func(i, total int, threadID string) {
	return func(i, t int, threadID string) {
		if t > total {
			total = t
		}
		fmt.Fprintf(os.Stdout, "  [%d/%d] %s\n", i, total, threadID)
	}
}

// printImportSummary prints the final import statistics.
func printImportSummary(result *langgraph.Result) {
	if outputJSON {
		type summary struct {
			ThreadsImported  int `json:"threads_imported"`
			ThreadsSkipped   int `json:"threads_skipped"`
			MessagesImported int `json:"messages_imported"`
		}
		_ = printJSON(summary{
			ThreadsImported:  result.ThreadsImported,
			ThreadsSkipped:   result.ThreadsSkipped,
			MessagesImported: result.MessagesImported,
		})
		return
	}

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Import complete:")
	fmt.Fprintf(os.Stdout, "  Threads imported:  %d\n", result.ThreadsImported)
	fmt.Fprintf(os.Stdout, "  Threads skipped:   %d\n", result.ThreadsSkipped)
	fmt.Fprintf(os.Stdout, "  Messages imported: %d\n", result.MessagesImported)

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\nErrors (%d):\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
	}
}

// readLangGraphExportFile opens and decodes a JSON export file into ExportData.
func readLangGraphExportFile(path string) (*langgraph.ExportData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %q: %w", path, err)
	}
	defer f.Close()

	var data langgraph.ExportData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %q: %w", path, err)
	}
	return &data, nil
}
