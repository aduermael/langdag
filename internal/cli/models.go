package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"langdag.com/langdag/internal/models"
)

var (
	modelsProvider string
	modelsUpdate   bool
)

// modelsCmd lists available models with pricing and capabilities.
var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List available models with pricing",
	Long: `Display model names, pricing (per 1M tokens), context windows, and max output
for all supported providers. Data is sourced from LiteLLM's model catalog.

Use --update to fetch the latest data from the remote source.`,
	Run: runModels,
}

func init() {
	modelsCmd.Flags().StringVarP(&modelsProvider, "provider", "p", "", "filter by provider (anthropic, openai, gemini, grok)")
	modelsCmd.Flags().BoolVar(&modelsUpdate, "update", false, "fetch latest model data from remote source")
	rootCmd.AddCommand(modelsCmd)
}

func runModels(cmd *cobra.Command, args []string) {
	cachePath := modelsCachePath()

	var catalog *models.Catalog
	var err error

	if modelsUpdate {
		fmt.Fprintln(os.Stderr, "Fetching latest model data...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		catalog, err = models.FetchLatest(ctx)
		if err != nil {
			exitError("failed to fetch model data: %v", err)
		}

		if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
			exitError("failed to create cache directory: %v", err)
		}
		if err := models.SaveCatalog(catalog, cachePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save cache: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Saved to %s\n", cachePath)
		}
	} else {
		catalog, err = models.LoadCatalog(cachePath)
		if err != nil {
			exitError("failed to load model catalog: %v", err)
		}
	}

	if outputJSON {
		printModelsJSON(catalog)
		return
	}

	providers := []string{"anthropic", "openai", "gemini", "grok"}
	if modelsProvider != "" {
		providers = []string{modelsProvider}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "MODEL\tINPUT $/1M\tOUTPUT $/1M\tCONTEXT\tMAX OUTPUT\n")

	for _, p := range providers {
		modelList := catalog.ForProvider(p)
		if len(modelList) == 0 {
			continue
		}
		for _, m := range modelList {
			fmt.Fprintf(w, "%s\t$%.4g\t$%.4g\t%s\t%s\n",
				m.ID,
				m.InputPricePer1M,
				m.OutputPricePer1M,
				formatTokens(m.ContextWindow),
				formatTokens(m.MaxOutput),
			)
		}
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\nSource: %s | Updated: %s\n", catalog.Source, catalog.UpdatedAt.Format("2006-01-02"))
}

func printModelsJSON(catalog *models.Catalog) {
	out := catalog
	if modelsProvider != "" {
		out = &models.Catalog{
			UpdatedAt: catalog.UpdatedAt,
			Source:    catalog.Source,
			Providers: map[string][]models.ModelPricing{
				modelsProvider: catalog.ForProvider(modelsProvider),
			},
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

func modelsCachePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "langdag", "model_catalog.json")
}

func formatTokens(n int) string {
	if n <= 0 {
		return "-"
	}
	if n >= 1_000_000 {
		if n%1_000_000 == 0 {
			return fmt.Sprintf("%dM", n/1_000_000)
		}
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		if n%1_000 == 0 {
			return fmt.Sprintf("%dK", n/1_000)
		}
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
