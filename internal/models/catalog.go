// Package models provides a catalog of LLM model pricing and capabilities.
package models

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"time"
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

// Catalog contains model information organized by provider.
type Catalog struct {
	UpdatedAt time.Time                `json:"updated_at"`
	Source    string                   `json:"source"`
	Providers map[string][]ModelPricing `json:"providers"`
}

// DefaultCatalog returns the embedded model catalog bundled with the library.
func DefaultCatalog() (*Catalog, error) {
	var catalog Catalog
	if err := json.Unmarshal(defaultCatalogJSON, &catalog); err != nil {
		return nil, fmt.Errorf("models: failed to parse embedded catalog: %w", err)
	}
	return &catalog, nil
}

// LoadCatalog loads the catalog from a cache file, falling back to the
// embedded default if the file does not exist or is invalid.
func LoadCatalog(cachePath string) (*Catalog, error) {
	if cachePath != "" {
		data, err := os.ReadFile(cachePath)
		if err == nil {
			var catalog Catalog
			if err := json.Unmarshal(data, &catalog); err == nil {
				return &catalog, nil
			}
		}
	}
	return DefaultCatalog()
}

// SaveCatalog writes the catalog to a JSON file.
func SaveCatalog(catalog *Catalog, path string) error {
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("models: failed to marshal catalog: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// ForProvider returns the models for a specific provider, or nil if not found.
func (c *Catalog) ForProvider(provider string) []ModelPricing {
	if c == nil || c.Providers == nil {
		return nil
	}
	if provider == "gemma" {
		provider = "gemini"
	}
	return c.Providers[provider]
}

// LookupModel finds a model by ID across all providers.
// Returns the model pricing and provider name, or false if not found.
func (c *Catalog) LookupModel(modelID string) (ModelPricing, string, bool) {
	if c == nil || c.Providers == nil {
		return ModelPricing{}, "", false
	}
	for provider, models := range c.Providers {
		for _, m := range models {
			if m.ID == modelID {
				return m, provider, true
			}
		}
	}
	return ModelPricing{}, "", false
}
