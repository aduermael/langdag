package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"langdag.com/langdag/internal/models"
)

var (
	modelsProvider   string
	modelsUpdate     bool
	modelsGenerate   bool
	modelsCatalogURL string
)

// modelsCmd lists available models with pricing and capabilities.
var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List available models with pricing",
	Long: `Display model names, pricing (per 1M tokens), context windows, and max output
for all supported providers. Data is sourced from official provider documentation.

Use --update to fetch the latest published catalog into the local runtime cache.
Use --generate --json to rebuild the deployment-aware catalog artifact for
publishing automation.`,
	Run: runModels,
}

func init() {
	modelsCmd.Flags().StringVarP(&modelsProvider, "provider", "p", "", "filter by model owner or deployment provider (anthropic, openai, google, xai, openrouter, ollama)")
	modelsCmd.Flags().BoolVar(&modelsUpdate, "update", false, "fetch latest published model catalog")
	modelsCmd.Flags().BoolVar(&modelsGenerate, "generate", false, "regenerate deployment-aware model catalog artifact")
	modelsCmd.Flags().StringVar(&modelsCatalogURL, "catalog-url", "", "published catalog URL for --update")
	rootCmd.AddCommand(modelsCmd)
}

func runModels(cmd *cobra.Command, args []string) {
	cachePath := modelsCachePath()

	var catalog *models.Catalog
	var err error

	if modelsGenerate && modelsUpdate {
		exitError("--generate and --update cannot be used together")
	}

	if modelsGenerate {
		if !outputJSON {
			exitError("--generate currently requires --json")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		catalog, err := models.FetchLatestV1(ctx)
		if err != nil {
			exitError("failed to fetch model data: %v", err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(catalog)
		return
	}

	if modelsUpdate {
		fmt.Fprintln(os.Stderr, "Fetching published model catalog...")
		opts := models.CatalogRefreshOptionsFromEnv(cachePath)
		if modelsCatalogURL != "" {
			opts.Endpoint = modelsCatalogURL
		}
		ctx := context.Background()
		result, refreshErr := models.RefreshCatalogCache(ctx, opts)
		if refreshErr != nil {
			exitError("failed to fetch published model catalog: %v", refreshErr)
		}
		if result.ReplacedCache {
			fmt.Fprintf(os.Stderr, "Saved to %s\n", cachePath)
		}
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(os.Stderr, "Diagnostic: %s: %s\n", diagnostic.Code, diagnostic.Message)
		}
		if result.Catalog == nil {
			exitError("published model catalog was not refreshed")
		}
		catalog = result.Catalog
	} else {
		result, loadErr := models.LoadCatalogWithOptions(models.CatalogLoadOptions{CachePath: cachePath})
		err = loadErr
		if err != nil {
			exitError("failed to load model catalog: %v", err)
		}
		catalog = result.Catalog
		if verbose {
			for _, diagnostic := range result.Diagnostics {
				fmt.Fprintf(os.Stderr, "Diagnostic: %s: %s\n", diagnostic.Code, diagnostic.Message)
			}
		}
	}

	if outputJSON {
		printModelsJSON(catalog)
		return
	}

	compiled, err := models.CompileCatalogV1(catalog)
	if err != nil {
		exitError("failed to compile model catalog: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "CANONICAL MODEL\tOWNER\tDEPLOYMENT\tAPI\tNATIVE MODEL\tINPUT $/1M\tOUTPUT $/1M\tCONTEXT\tMAX OUTPUT\n")

	for _, offering := range catalog.Offerings {
		model := compiled.ModelsByID[offering.CanonicalModelID]
		deployment := compiled.DeploymentsByID[offering.DeploymentID]
		if model == nil || deployment == nil {
			continue
		}
		if modelsProvider != "" && !catalogFilterMatches(compiled, model, deployment, offering.DeploymentID, modelsProvider) {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			offering.CanonicalModelID,
			model.ProviderID,
			offering.DeploymentID,
			deployment.APIProtocolID,
			offering.NativeModelID,
			formatCatalogPrice(offering.Pricing, "input_tokens"),
			formatCatalogPrice(offering.Pricing, "output_tokens"),
			formatTokens(model.ContextWindow),
			formatTokens(model.MaxOutput),
		)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\nGenerated: %s | Stale after: %s\n", catalog.GeneratedAt.Format("2006-01-02"), catalog.StaleAfter.Format("2006-01-02"))
	for _, diagnostic := range compiled.Diagnostics {
		fmt.Fprintf(os.Stderr, "Diagnostic: %s: %s\n", diagnostic.Code, diagnostic.Message)
	}
}

func printModelsJSON(catalog *models.Catalog) {
	out := catalog
	if modelsProvider != "" {
		if compiled, err := models.CompileCatalogV1(catalog); err == nil {
			out = filterCatalogForJSON(catalog, compiled, modelsProvider)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

func modelsCachePath() string {
	return models.DefaultCatalogCachePath()
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

func formatCatalogPrice(pricing models.PricingV1, dimension string) string {
	switch pricing.Status {
	case models.PricingKnown, models.PricingPartial, models.PricingFree:
		if rate, ok := pricing.RatesPer1M[dimension]; ok {
			return fmt.Sprintf("$%.4g", rate)
		}
		if pricing.Status == models.PricingPartial {
			return "partial"
		}
		return pricingStatusLabel(pricing.Status)
	default:
		return pricingStatusLabel(pricing.Status)
	}
}

func pricingStatusLabel(status models.PricingStatus) string {
	if status == "" {
		return "unknown"
	}
	return string(status)
}

func filterCatalogForJSON(catalog *models.Catalog, compiled *models.CompiledCatalogV1, filter string) *models.Catalog {
	out := &models.Catalog{
		SchemaVersion:     catalog.SchemaVersion,
		GeneratedAt:       catalog.GeneratedAt,
		StaleAfter:        catalog.StaleAfter,
		Providers:         map[string]*models.ProviderV1{},
		APIProtocols:      map[string]*models.APIProtocolV1{},
		Deployments:       map[string]*models.DeploymentV1{},
		Models:            map[string]*models.ModelV1{},
		Offerings:         []models.ModelOfferingV1{},
		OfferingTemplates: []models.ModelOfferingTemplateV1{},
		Aliases:           map[string]string{},
		Provenance:        catalog.Provenance,
	}
	includeDeployment := func(deployment *models.DeploymentV1) {
		if deployment == nil {
			return
		}
		out.Deployments[deployment.ID] = deployment
		if provider := compiled.ProvidersByID[deployment.ProviderID]; provider != nil {
			out.Providers[provider.ID] = provider
		}
		if protocol := compiled.ProtocolsByID[deployment.APIProtocolID]; protocol != nil {
			out.APIProtocols[protocol.ID] = protocol
		}
	}
	includeModel := func(model *models.ModelV1) {
		if model == nil {
			return
		}
		out.Models[model.ID] = model
		if provider := compiled.ProvidersByID[model.ProviderID]; provider != nil {
			out.Providers[provider.ID] = provider
		}
	}
	for _, offering := range catalog.Offerings {
		model := compiled.ModelsByID[offering.CanonicalModelID]
		deployment := compiled.DeploymentsByID[offering.DeploymentID]
		if !catalogFilterMatches(compiled, model, deployment, offering.DeploymentID, filter) {
			continue
		}
		out.Offerings = append(out.Offerings, offering)
		includeModel(model)
		includeDeployment(deployment)
	}
	for _, template := range catalog.OfferingTemplates {
		model := compiled.ModelsByID[template.CanonicalModelID]
		deployment := compiled.DeploymentsByID[template.DeploymentID]
		if !catalogFilterMatches(compiled, model, deployment, template.DeploymentID, filter) {
			continue
		}
		out.OfferingTemplates = append(out.OfferingTemplates, template)
		includeModel(model)
		includeDeployment(deployment)
	}
	for alias, target := range catalog.Aliases {
		if out.Models[target] != nil {
			out.Aliases[alias] = target
		}
	}
	if len(out.Offerings) == 0 && len(out.OfferingTemplates) > 0 {
		for _, offering := range catalog.Offerings {
			if out.Models[offering.CanonicalModelID] == nil {
				continue
			}
			model := compiled.ModelsByID[offering.CanonicalModelID]
			deployment := compiled.DeploymentsByID[offering.DeploymentID]
			if model == nil || deployment == nil {
				continue
			}
			out.Offerings = append(out.Offerings, offering)
			includeDeployment(deployment)
		}
	}
	if len(out.Offerings) == 0 {
		return catalog
	}
	return out
}

func catalogFilterMatches(compiled *models.CompiledCatalogV1, model *models.ModelV1, deployment *models.DeploymentV1, deploymentID, filter string) bool {
	if model == nil || deployment == nil {
		return false
	}
	if model.ProviderID == filter || deployment.ProviderID == filter || deploymentID == filter {
		return true
	}
	if providerHasAlias(compiled.ProvidersByID[model.ProviderID], filter) {
		return true
	}
	if providerHasAlias(compiled.ProvidersByID[deployment.ProviderID], filter) {
		return true
	}
	for _, legacyDeploymentID := range legacyFilterDeployments(filter) {
		if deploymentID == legacyDeploymentID {
			return true
		}
	}
	return false
}

func providerHasAlias(provider *models.ProviderV1, alias string) bool {
	if provider == nil {
		return false
	}
	for _, candidate := range provider.Aliases {
		if candidate == alias {
			return true
		}
	}
	return false
}

func legacyFilterDeployments(filter string) []string {
	switch filter {
	case "anthropic":
		return []string{"anthropic-direct"}
	case "openai":
		return []string{"openai-direct"}
	case "gemini", "gemma":
		return []string{"gemini-direct"}
	case "grok":
		return []string{"grok-direct"}
	case "ollama":
		return []string{"ollama-local"}
	default:
		return []string{filter}
	}
}
