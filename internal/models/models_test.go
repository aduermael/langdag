package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCatalog(t *testing.T) {
	catalog, err := DefaultCatalog()
	if err != nil {
		t.Fatalf("DefaultCatalog() error: %v", err)
	}

	if catalog.Source != "litellm" {
		t.Errorf("Source = %q, want %q", catalog.Source, "litellm")
	}

	expectedProviders := []string{"anthropic", "openai", "gemini", "grok"}
	for _, p := range expectedProviders {
		models := catalog.ForProvider(p)
		if len(models) == 0 {
			t.Errorf("provider %q has no models", p)
		}
	}
}

func TestDefaultCatalog_KnownModels(t *testing.T) {
	catalog, err := DefaultCatalog()
	if err != nil {
		t.Fatalf("DefaultCatalog() error: %v", err)
	}

	tests := []struct {
		modelID        string
		wantProvider   string
		wantInputGt0   bool
		wantCtxWindow  int
	}{
		{"claude-sonnet-4-20250514", "anthropic", true, 0},
		{"gpt-4o", "openai", true, 128000},
		{"gemini-2.5-flash", "gemini", true, 0},
		{"grok-3", "grok", true, 131072},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			m, provider, ok := catalog.LookupModel(tt.modelID)
			if !ok {
				t.Fatalf("model %q not found in catalog", tt.modelID)
			}
			if provider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", provider, tt.wantProvider)
			}
			if tt.wantInputGt0 && m.InputPricePer1M <= 0 {
				t.Errorf("InputPricePer1M = %f, want > 0", m.InputPricePer1M)
			}
			if tt.wantCtxWindow > 0 && m.ContextWindow != tt.wantCtxWindow {
				t.Errorf("ContextWindow = %d, want %d", m.ContextWindow, tt.wantCtxWindow)
			}
			if m.MaxOutput <= 0 {
				t.Errorf("MaxOutput = %d, want > 0", m.MaxOutput)
			}
		})
	}
}

func TestLookupModel_NotFound(t *testing.T) {
	catalog, err := DefaultCatalog()
	if err != nil {
		t.Fatalf("DefaultCatalog() error: %v", err)
	}

	_, _, ok := catalog.LookupModel("nonexistent-model")
	if ok {
		t.Error("expected LookupModel to return false for nonexistent model")
	}
}

func TestForProvider_Unknown(t *testing.T) {
	catalog, err := DefaultCatalog()
	if err != nil {
		t.Fatalf("DefaultCatalog() error: %v", err)
	}

	models := catalog.ForProvider("unknown-provider")
	if models != nil {
		t.Errorf("expected nil for unknown provider, got %d models", len(models))
	}
}

func TestForProvider_NilCatalog(t *testing.T) {
	var catalog *Catalog
	models := catalog.ForProvider("anthropic")
	if models != nil {
		t.Error("expected nil from nil catalog")
	}
}

func TestLoadCatalog_FallsBackToDefault(t *testing.T) {
	catalog, err := LoadCatalog("/nonexistent/path/catalog.json")
	if err != nil {
		t.Fatalf("LoadCatalog() error: %v", err)
	}

	if catalog.Source != "litellm" {
		t.Errorf("Source = %q, want %q", catalog.Source, "litellm")
	}
}

func TestLoadCatalog_EmptyPath(t *testing.T) {
	catalog, err := LoadCatalog("")
	if err != nil {
		t.Fatalf("LoadCatalog(\"\") error: %v", err)
	}

	if len(catalog.Providers) == 0 {
		t.Error("expected non-empty providers from default catalog")
	}
}

func TestSaveAndLoadCatalog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")

	original, err := DefaultCatalog()
	if err != nil {
		t.Fatalf("DefaultCatalog() error: %v", err)
	}

	if err := SaveCatalog(original, path); err != nil {
		t.Fatalf("SaveCatalog() error: %v", err)
	}

	loaded, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog() error: %v", err)
	}

	if loaded.Source != original.Source {
		t.Errorf("Source = %q, want %q", loaded.Source, original.Source)
	}

	for provider, originalModels := range original.Providers {
		loadedModels := loaded.Providers[provider]
		if len(loadedModels) != len(originalModels) {
			t.Errorf("provider %q: got %d models, want %d", provider, len(loadedModels), len(originalModels))
		}
	}
}

func TestLoadCatalog_PrefersCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")

	custom := &Catalog{
		Source: "custom",
		Providers: map[string][]ModelPricing{
			"test": {{ID: "test-model", InputPricePer1M: 1.0, OutputPricePer1M: 2.0, ContextWindow: 1000, MaxOutput: 500}},
		},
	}

	if err := SaveCatalog(custom, path); err != nil {
		t.Fatalf("SaveCatalog() error: %v", err)
	}

	loaded, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog() error: %v", err)
	}

	if loaded.Source != "custom" {
		t.Errorf("Source = %q, want %q (cache should take precedence)", loaded.Source, "custom")
	}
}

func TestLoadCatalog_InvalidCacheFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")

	if err := os.WriteFile(path, []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog() error: %v", err)
	}

	if catalog.Source != "litellm" {
		t.Errorf("Source = %q, want %q (should fall back to default)", catalog.Source, "litellm")
	}
}

func TestParseLiteLLMData(t *testing.T) {
	// Minimal LiteLLM-format test data
	testData := map[string]interface{}{
		"claude-sonnet-4-6": map[string]interface{}{
			"litellm_provider":      "anthropic",
			"mode":                  "chat",
			"input_cost_per_token":  0.000003,
			"output_cost_per_token": 0.000015,
			"max_input_tokens":      200000,
			"max_output_tokens":     64000,
		},
		"gpt-4o": map[string]interface{}{
			"litellm_provider":      "openai",
			"mode":                  "chat",
			"input_cost_per_token":  0.0000025,
			"output_cost_per_token": 0.00001,
			"max_input_tokens":      128000,
			"max_output_tokens":     16384,
		},
		"gemini/gemini-2.5-flash": map[string]interface{}{
			"litellm_provider":      "gemini",
			"mode":                  "chat",
			"input_cost_per_token":  0.0000003,
			"output_cost_per_token": 0.0000025,
			"max_input_tokens":      1048576,
			"max_output_tokens":     65535,
		},
		"xai/grok-3": map[string]interface{}{
			"litellm_provider":      "xai",
			"mode":                  "chat",
			"input_cost_per_token":  0.000003,
			"output_cost_per_token": 0.000015,
			"max_input_tokens":      131072,
			"max_output_tokens":     131072,
		},
		// Should be filtered out: wrong mode
		"text-embedding-3-small": map[string]interface{}{
			"litellm_provider":      "openai",
			"mode":                  "embedding",
			"input_cost_per_token":  0.00000002,
			"output_cost_per_token": 0.0,
			"max_input_tokens":      8191,
			"max_output_tokens":     0,
		},
		// Should be filtered out: unsupported provider
		"mistral-large-latest": map[string]interface{}{
			"litellm_provider":      "mistral",
			"mode":                  "chat",
			"input_cost_per_token":  0.000002,
			"output_cost_per_token": 0.000006,
			"max_input_tokens":      128000,
			"max_output_tokens":     8192,
		},
	}

	data, err := json.Marshal(testData)
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := parseLiteLLMData(data)
	if err != nil {
		t.Fatalf("parseLiteLLMData() error: %v", err)
	}

	// Check we have exactly 4 providers
	if len(catalog.Providers) != 4 {
		t.Errorf("got %d providers, want 4", len(catalog.Providers))
	}

	// Verify Anthropic
	anthropic := catalog.ForProvider("anthropic")
	if len(anthropic) != 1 {
		t.Fatalf("anthropic: got %d models, want 1", len(anthropic))
	}
	if anthropic[0].ID != "claude-sonnet-4-6" {
		t.Errorf("anthropic model ID = %q, want %q", anthropic[0].ID, "claude-sonnet-4-6")
	}
	if anthropic[0].InputPricePer1M != 3.0 {
		t.Errorf("InputPricePer1M = %f, want 3.0", anthropic[0].InputPricePer1M)
	}
	if anthropic[0].OutputPricePer1M != 15.0 {
		t.Errorf("OutputPricePer1M = %f, want 15.0", anthropic[0].OutputPricePer1M)
	}

	// Verify Gemini prefix was stripped
	gemini := catalog.ForProvider("gemini")
	if len(gemini) != 1 {
		t.Fatalf("gemini: got %d models, want 1", len(gemini))
	}
	if gemini[0].ID != "gemini-2.5-flash" {
		t.Errorf("gemini model ID = %q, want %q (prefix should be stripped)", gemini[0].ID, "gemini-2.5-flash")
	}

	// Verify xAI prefix was stripped and mapped to "grok"
	grok := catalog.ForProvider("grok")
	if len(grok) != 1 {
		t.Fatalf("grok: got %d models, want 1", len(grok))
	}
	if grok[0].ID != "grok-3" {
		t.Errorf("grok model ID = %q, want %q (prefix should be stripped)", grok[0].ID, "grok-3")
	}

	// Verify embedding model was filtered out
	for _, models := range catalog.Providers {
		for _, m := range models {
			if m.ID == "text-embedding-3-small" {
				t.Error("embedding model should have been filtered out")
			}
		}
	}

	// Verify mistral was filtered out
	if catalog.ForProvider("mistral") != nil {
		t.Error("mistral provider should not be present")
	}
}

func TestFetchLatest(t *testing.T) {
	// Create a test server with minimal LiteLLM-format data
	testData := map[string]interface{}{
		"claude-sonnet-4-6": map[string]interface{}{
			"litellm_provider":      "anthropic",
			"mode":                  "chat",
			"input_cost_per_token":  0.000003,
			"output_cost_per_token": 0.000015,
			"max_input_tokens":      200000,
			"max_output_tokens":     64000,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(testData)
	}))
	defer server.Close()

	// Override the URL for testing
	origURL := fetchURL
	defer func() { setLiteLLMURL(origURL) }()
	setLiteLLMURL(server.URL)

	catalog, err := FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest() error: %v", err)
	}

	if catalog.Source != "litellm" {
		t.Errorf("Source = %q, want %q", catalog.Source, "litellm")
	}

	anthropic := catalog.ForProvider("anthropic")
	if len(anthropic) != 1 {
		t.Fatalf("anthropic: got %d models, want 1", len(anthropic))
	}
}

func TestFetchLatest_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origURL := fetchURL
	defer func() { setLiteLLMURL(origURL) }()
	setLiteLLMURL(server.URL)

	_, err := FetchLatest(context.Background())
	if err == nil {
		t.Error("expected error for server error response")
	}
}
