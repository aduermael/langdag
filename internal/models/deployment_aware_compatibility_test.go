package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOldProviderKeyedCatalogCacheFixtureLoads(t *testing.T) {
	path := filepath.Join("testdata", "deployment_aware_compatibility", "old_provider_keyed_catalog_cache.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture LegacyCatalog
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if got := fixture.UpdatedAt.Format("2006-01-02"); got != "2026-05-01" {
		t.Fatalf("fixture UpdatedAt = %q, want sentinel 2026-05-01", got)
	}

	catalog, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if !catalog.GeneratedAt.Equal(fixture.UpdatedAt) {
		t.Fatalf("LoadCatalog did not load the fixture; GeneratedAt = %s, want %s", catalog.GeneratedAt, fixture.UpdatedAt)
	}
	if catalog.SchemaVersion != CatalogV1SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", catalog.SchemaVersion, CatalogV1SchemaVersion)
	}

	for _, provider := range []string{"anthropic", "openai", "gemini", "gemini-vertex", "grok"} {
		if len(catalog.ForProvider(provider)) == 0 {
			t.Errorf("provider %q missing from old provider-keyed cache fixture", provider)
		}
	}
	if catalog.ForProvider("gemini")[0].ID != catalog.ForProvider("gemini-vertex")[0].ID {
		t.Errorf("fixture should include a duplicated native model ID across provider keys")
	}

	model, provider, ok := catalog.LookupModel("gpt-4.1-2025-04-14")
	if !ok {
		t.Fatal("gpt-4.1-2025-04-14 missing from old cache fixture")
	}
	if provider != "openai" {
		t.Errorf("provider = %q, want openai", provider)
	}
	if model.InputPricePer1M != 2 || model.OutputPricePer1M != 8 {
		t.Errorf("pricing = %f/%f, want 2/8", model.InputPricePer1M, model.OutputPricePer1M)
	}
	if len(model.ServerTools) != 1 || model.ServerTools[0] != "web_search" {
		t.Errorf("ServerTools = %v, want [web_search]", model.ServerTools)
	}
}

func TestLookupModelIncludesLegacyDeploymentProviderKeys(t *testing.T) {
	legacy := &LegacyCatalog{
		UpdatedAt: mustParseTime(t, "2026-05-01T00:00:00Z"),
		Source:    "providers",
		Providers: map[string][]ModelPricing{
			"anthropic-bedrock": {
				{ID: "anthropic.claude-sonnet-4-20250514-v1:0", InputPricePer1M: 3, OutputPricePer1M: 15, ContextWindow: 200000},
			},
			"anthropic-vertex": {
				{ID: "claude-sonnet-4@20250514", InputPricePer1M: 3, OutputPricePer1M: 15, ContextWindow: 200000},
			},
			"openai-azure": {
				{ID: "my-gpt-4-1-prod", InputPricePer1M: 2, OutputPricePer1M: 8, ContextWindow: 1048576},
			},
		},
	}
	catalog := CatalogV1FromLegacyCatalog(legacy)

	tests := []struct {
		modelID      string
		wantProvider string
	}{
		{"anthropic.claude-sonnet-4-20250514-v1:0", "anthropic-bedrock"},
		{"claude-sonnet-4@20250514", "anthropic-vertex"},
		{"my-gpt-4-1-prod", "openai-azure"},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			_, provider, ok := catalog.LookupModel(tt.modelID)
			if !ok {
				t.Fatalf("LookupModel(%q) did not find legacy deployment-keyed offering", tt.modelID)
			}
			if provider != tt.wantProvider {
				t.Fatalf("provider = %q, want %q", provider, tt.wantProvider)
			}
		})
	}
}

func TestOpenRouterLegacyMigrationDoesNotLeakOwner(t *testing.T) {
	legacy := &LegacyCatalog{
		UpdatedAt: mustParseTime(t, "2026-05-01T00:00:00Z"),
		Source:    "providers",
		Providers: map[string][]ModelPricing{
			"openrouter": {
				{ID: "anthropic/claude-sonnet-4.5", InputPricePer1M: 3, OutputPricePer1M: 15, ContextWindow: 200000},
				{ID: "unqualified-openrouter-model", InputPricePer1M: 1, OutputPricePer1M: 2, ContextWindow: 100000},
			},
		},
	}

	catalog := CatalogV1FromLegacyCatalog(legacy)
	if catalog.Models["anthropic/claude-sonnet-4.5"] == nil {
		t.Fatal("slash-qualified OpenRouter model did not keep owner-qualified canonical ID")
	}
	model := catalog.Models["openrouter/unqualified-openrouter-model"]
	if model == nil {
		t.Fatal("unqualified OpenRouter model missing openrouter canonical ID")
	}
	if model.ProviderID != "openrouter" {
		t.Fatalf("unqualified OpenRouter provider_id = %q, want openrouter", model.ProviderID)
	}
	if got := catalog.Aliases["unqualified-openrouter-model"]; got != "openrouter/unqualified-openrouter-model" {
		t.Fatalf("alias = %q, want openrouter/unqualified-openrouter-model", got)
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}
