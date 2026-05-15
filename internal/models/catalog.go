// Package models provides a catalog of LLM model pricing and capabilities.
package models

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"langdag.com/langdag/types"
)

//go:embed catalog.json
var defaultCatalogJSON []byte

// ModelPricing contains pricing and capability information for a model.
type ModelPricing struct {
	ID               string   `json:"id"`
	InputPricePer1M  float64  `json:"input_price_per_1m"`
	OutputPricePer1M float64  `json:"output_price_per_1m"`
	ContextWindow    int      `json:"context_window"`
	MaxOutput        int      `json:"max_output"`
	ServerTools      []string `json:"server_tools,omitempty"`
}

// LegacyCatalog is the pre-v1 provider-keyed catalog shape. It is kept only so
// old embedded catalogs and cache files can be migrated into CatalogV1.
type LegacyCatalog struct {
	UpdatedAt time.Time                 `json:"updated_at"`
	Source    string                    `json:"source"`
	Providers map[string][]ModelPricing `json:"providers"`
}

// Catalog is the deployment-aware model catalog v1 shape.
type Catalog = CatalogV1

// DefaultCatalog returns the embedded model catalog bundled with the library.
func DefaultCatalog() (*Catalog, error) {
	catalog, err := ParseCatalogV1(defaultCatalogJSON)
	if err != nil {
		return nil, fmt.Errorf("models: failed to parse embedded catalog: %w", err)
	}
	NormalizeCatalogV1(catalog)
	return catalog, nil
}

// LoadCatalog loads the catalog from a cache file, falling back to the
// embedded default if the file does not exist or is invalid.
func LoadCatalog(cachePath string) (*Catalog, error) {
	if cachePath != "" {
		data, err := os.ReadFile(cachePath)
		if err == nil {
			catalog, err := ParseCatalogV1(data)
			if err == nil {
				NormalizeCatalogV1(catalog)
				return catalog, nil
			}
		}
	}
	return DefaultCatalog()
}

// SaveCatalog writes the catalog to a JSON file.
func SaveCatalog(catalog *Catalog, path string) error {
	if catalog == nil {
		return fmt.Errorf("models: cannot save nil catalog")
	}
	NormalizeCatalogV1(catalog)
	if err := ValidateCatalogV1(catalog); err != nil {
		return fmt.Errorf("models: invalid catalog: %w", err)
	}
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("models: failed to marshal catalog: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// ForProvider returns a legacy provider view over routeable deployment
// offerings. Herm still uses this until it switches its picker to canonical
// model rows in a later phase.
func (c *CatalogV1) ForProvider(provider string) []ModelPricing {
	if c == nil {
		return nil
	}
	deployments := legacyProviderDeploymentsV1(provider)
	if len(deployments) == 0 {
		return nil
	}
	compiled, err := CompileCatalogV1(c)
	if err != nil {
		return nil
	}
	var out []ModelPricing
	for _, deploymentID := range deployments {
		for _, offering := range compiled.OfferingsByDeployment[deploymentID] {
			out = append(out, modelPricingFromOfferingV1(compiled, offering))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LookupModel finds a model by ID across all providers.
// Returns the model pricing and provider name, or false if not found.
func (c *CatalogV1) LookupModel(modelID string) (ModelPricing, string, bool) {
	if c == nil {
		return ModelPricing{}, "", false
	}
	compiled, err := CompileCatalogV1(c)
	if err != nil {
		return ModelPricing{}, "", false
	}
	for _, provider := range []string{"anthropic", "anthropic-bedrock", "anthropic-vertex", "openai", "openai-azure", "gemini", "gemini-vertex", "grok", "openrouter", "ollama"} {
		for _, deploymentID := range legacyProviderDeploymentsV1(provider) {
			for _, offering := range compiled.OfferingsByDeployment[deploymentID] {
				if offering.NativeModelID == modelID || offering.CanonicalModelID == modelID {
					return modelPricingFromOfferingV1(compiled, offering), provider, true
				}
			}
		}
	}
	return ModelPricing{}, "", false
}

func strictJSONUnmarshal(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON")
		}
		return err
	}
	return nil
}

func legacyProviderDeploymentsV1(provider string) []string {
	switch provider {
	case "anthropic":
		return []string{"anthropic-direct"}
	case "anthropic-bedrock":
		return []string{"anthropic-bedrock"}
	case "anthropic-vertex":
		return []string{"anthropic-vertex"}
	case "openai":
		return []string{"openai-direct"}
	case "openai-azure":
		return []string{"openai-azure"}
	case "gemini", "gemma":
		return []string{"gemini-direct"}
	case "gemini-vertex":
		return []string{"gemini-vertex"}
	case "grok":
		return []string{"grok-direct"}
	case "openrouter":
		return []string{"openrouter"}
	case "ollama":
		return []string{"ollama-local"}
	default:
		return nil
	}
}

func modelPricingFromOfferingV1(compiled *CompiledCatalogV1, offering *ModelOfferingV1) ModelPricing {
	model := compiled.ModelsByID[offering.CanonicalModelID]
	pricing := offering.Pricing.RatesPer1M
	out := ModelPricing{
		ID:               offering.NativeModelID,
		InputPricePer1M:  pricing["input_tokens"],
		OutputPricePer1M: pricing["output_tokens"],
	}
	if model != nil {
		out.ContextWindow = model.ContextWindow
		out.MaxOutput = model.MaxOutput
	}
	for tool, state := range offering.Capabilities.ServerTools {
		if state == CapabilitySupported {
			out.ServerTools = append(out.ServerTools, tool)
		}
	}
	sort.Strings(out.ServerTools)
	return out
}

// MetadataForLegacyProviderModel returns best-effort deployment-aware metadata
// for legacy callers that still pass provider/native model IDs directly. The
// routing phase replaces this with explicit offering resolution before adapter
// execution, but this keeps Phase 3 persistence typed and deterministic.
func (c *CatalogV1) MetadataForLegacyProviderModel(providerName, requestedModel, responseModel string) (*types.ModelResolutionMetadata, *types.PricingSnapshot, bool) {
	if c == nil {
		return legacyResolutionOnly(providerName, requestedModel, responseModel), nil, false
	}
	compiled, err := CompileCatalogV1(c)
	if err != nil {
		return legacyResolutionOnly(providerName, requestedModel, responseModel), nil, false
	}
	for _, modelID := range compactStrings(responseModel, requestedModel) {
		if offering, ok := compiled.OfferingForLegacyProviderModel(providerName, modelID); ok {
			resolution := ModelResolutionMetadataFromOffering(offering)
			snapshot := PricingSnapshotFromPricingV1(offering.Pricing)
			return &resolution, &snapshot, true
		}
	}
	return legacyResolutionOnly(providerName, requestedModel, responseModel), nil, false
}

func (c *CompiledCatalogV1) OfferingForLegacyProviderModel(providerName, modelID string) (*ModelOfferingV1, bool) {
	if c == nil || modelID == "" {
		return nil, false
	}
	deployments := legacyProviderDeploymentsV1(providerName)
	if len(deployments) == 0 {
		if c.Catalog == nil {
			return nil, false
		}
		for i := range c.Catalog.Offerings {
			offering := &c.Catalog.Offerings[i]
			if offering.NativeModelID == modelID || offering.CanonicalModelID == modelID {
				return offering, true
			}
		}
		return nil, false
	}
	for _, deploymentID := range deployments {
		for _, offering := range c.OfferingsByDeployment[deploymentID] {
			if offering.NativeModelID == modelID || offering.CanonicalModelID == modelID {
				return offering, true
			}
		}
	}
	return nil, false
}

func ModelResolutionMetadataFromOffering(offering *ModelOfferingV1) types.ModelResolutionMetadata {
	if offering == nil {
		return types.ModelResolutionMetadata{}
	}
	resolution := types.ModelResolutionMetadata{
		CanonicalModelID: offering.CanonicalModelID,
		OfferingID:       offering.ID,
		DeploymentID:     offering.DeploymentID,
		NativeModelID:    offering.NativeModelID,
	}
	if offering.Model != nil {
		resolution.ProviderID = offering.Model.ProviderID
	}
	if resolution.ProviderID == "" && offering.Provider != nil {
		resolution.ProviderID = offering.Provider.ID
	}
	if resolution.ProviderID == "" {
		if owner, _, ok := strings.Cut(offering.CanonicalModelID, "/"); ok {
			resolution.ProviderID = owner
		}
	}
	if resolution.APIProtocolID == "" && offering.APIProtocol != nil {
		resolution.APIProtocolID = offering.APIProtocol.ID
	}
	if resolution.APIProtocolID == "" && offering.Deployment != nil {
		resolution.APIProtocolID = offering.Deployment.APIProtocolID
	}
	return resolution
}

func PricingSnapshotFromPricingV1(pricing PricingV1) types.PricingSnapshot {
	source := types.CostSourceCatalog
	if pricing.Source != "" && pricing.Source != "catalog" {
		source = types.CostSource(pricing.Source)
	}
	return types.PricingSnapshot{
		Status:            types.CostStatus(pricing.Status),
		Currency:          pricing.Currency,
		EffectiveAt:       pricing.EffectiveAt,
		Source:            source,
		RatesPer1M:        cloneFloat64Map(pricing.RatesPer1M),
		MissingDimensions: append([]string(nil), pricing.MissingDimensions...),
	}
}

func legacyResolutionOnly(providerName, requestedModel, responseModel string) *types.ModelResolutionMetadata {
	nativeID := responseModel
	if nativeID == "" {
		nativeID = requestedModel
	}
	if nativeID == "" && providerName == "" {
		return nil
	}
	resolution := &types.ModelResolutionMetadata{NativeModelID: nativeID}
	if strings.Contains(nativeID, "/") {
		resolution.CanonicalModelID = nativeID
	}
	if deployments := legacyProviderDeploymentsV1(providerName); len(deployments) > 0 {
		resolution.DeploymentID = deployments[0]
	}
	return resolution
}

func compactStrings(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cloneFloat64Map(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func NormalizeCatalogV1(catalog *CatalogV1) {
	if catalog == nil {
		return
	}
	for _, provider := range catalog.Providers {
		if provider != nil {
			sort.Strings(provider.Aliases)
		}
	}
	for _, model := range catalog.Models {
		if model != nil {
			sort.Strings(model.Aliases)
		}
	}
	sort.SliceStable(catalog.Offerings, func(i, j int) bool {
		if catalog.Offerings[i].CanonicalModelID != catalog.Offerings[j].CanonicalModelID {
			return catalog.Offerings[i].CanonicalModelID < catalog.Offerings[j].CanonicalModelID
		}
		return catalog.Offerings[i].ID < catalog.Offerings[j].ID
	})
	sort.SliceStable(catalog.OfferingTemplates, func(i, j int) bool {
		if catalog.OfferingTemplates[i].CanonicalModelID != catalog.OfferingTemplates[j].CanonicalModelID {
			return catalog.OfferingTemplates[i].CanonicalModelID < catalog.OfferingTemplates[j].CanonicalModelID
		}
		return catalog.OfferingTemplates[i].ID < catalog.OfferingTemplates[j].ID
	})
	for i := range catalog.Offerings {
		normalizePricingV1(&catalog.Offerings[i].Pricing)
	}
	for i := range catalog.OfferingTemplates {
		normalizePricingV1(&catalog.OfferingTemplates[i].Pricing)
	}
}

func normalizePricingV1(pricing *PricingV1) {
	if pricing == nil {
		return
	}
	sort.Strings(pricing.MissingDimensions)
	sort.Strings(pricing.Notes)
}
