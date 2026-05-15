package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPhase0OldProviderKeyedCatalogCacheFixtureLoads(t *testing.T) {
	path := filepath.Join("testdata", "deployment_aware_phase0", "old_provider_keyed_catalog_cache.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture Catalog
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
	if !catalog.UpdatedAt.Equal(fixture.UpdatedAt) {
		t.Fatalf("LoadCatalog did not load the fixture; UpdatedAt = %s, want %s", catalog.UpdatedAt, fixture.UpdatedAt)
	}
	if catalog.Source != "providers" {
		t.Fatalf("Source = %q, want providers", catalog.Source)
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
