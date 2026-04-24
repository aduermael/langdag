package models

import (
	"context"
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

	if catalog.Source != "providers" {
		t.Errorf("Source = %q, want %q", catalog.Source, "providers")
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
		modelID      string
		wantProvider string
		wantInputGt0 bool
		wantCtxGt0   bool
	}{
		{"gpt-4o-2024-08-06", "openai", true, true},
		{"grok-3", "grok", true, true},
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
			if tt.wantCtxGt0 && m.ContextWindow <= 0 {
				t.Errorf("ContextWindow = %d, want > 0", m.ContextWindow)
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

	if catalog.Source != "providers" {
		t.Errorf("Source = %q, want %q", catalog.Source, "providers")
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

	if catalog.Source != "providers" {
		t.Errorf("Source = %q, want %q (should fall back to default)", catalog.Source, "providers")
	}
}

func TestFetchLatest(t *testing.T) {
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`# Models

### gpt-4o-2024-08-06

- Context window size: 128000
- Maximum output tokens: 16384

## Text tokens

| Name | Input | Cached input | Output | Unit |
| --- | --- | --- | --- | --- |
| gpt-4o | 2.5 | 1.25 | 10 | 1M tokens |
| gpt-4o-2024-08-06 | 2.5 | 1.25 | 10 | 1M tokens |
`))
	}))
	defer openAIServer.Close()

	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<table>
<tr><th>Feature</th><th>Model</th></tr>
<tr><td>Claude API ID</td><td>claude-sonnet-4-6</td></tr>
<tr><td>Pricing</td><td>$3 / input MTok $15 / output MTok</td></tr>
<tr><td>Context window</td><td>200K tokens</td></tr>
<tr><td>Max output</td><td>64K tokens</td></tr>
</table>`))
	}))
	defer anthropicServer.Close()

	// Gemini: pricing page + spec page
	geminiMux := http.NewServeMux()
	geminiMux.HandleFunc("/pricing", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<h3>Gemini 3 Flash Preview</h3><p>Input price $0.50</p><p>Output price $3.00</p>`))
	})
	geminiMux.HandleFunc("/models/gemini-3-flash-preview", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>Input token limit 1,048,576 Output token limit 65,536</html>`))
	})
	geminiServer := httptest.NewServer(geminiMux)
	defer geminiServer.Close()

	grokServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`\"name\":\"grok-3\",\"promptTextTokenPrice\":\"$n30000\",\"completionTextTokenPrice\":\"$n150000\",\"maxPromptLength\":131072`))
	}))
	defer grokServer.Close()

	origOpenAI, origAnthropic, origGemini, origGeminiSpec, origGrok := openAISourceURL, anthropicSourceURL, geminiSourceURL, geminiSpecBaseURL, grokSourceURL
	defer func() {
		openAISourceURL = origOpenAI
		anthropicSourceURL = origAnthropic
		geminiSourceURL = origGemini
		geminiSpecBaseURL = origGeminiSpec
		grokSourceURL = origGrok
	}()
	openAISourceURL = openAIServer.URL
	anthropicSourceURL = anthropicServer.URL
	geminiSourceURL = geminiServer.URL + "/pricing"
	geminiSpecBaseURL = geminiServer.URL + "/models"
	grokSourceURL = grokServer.URL

	catalog, err := FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest() error: %v", err)
	}

	if catalog.Source != "providers" {
		t.Errorf("Source = %q, want %q", catalog.Source, "providers")
	}

	// Check at least one model per provider
	for _, p := range []string{"openai", "anthropic", "gemini", "grok"} {
		if len(catalog.ForProvider(p)) == 0 {
			t.Errorf("no models for provider %q", p)
		}
	}

	// gpt-4o-2024-08-06 should have full data (pricing + context from same file)
	m, _, ok := catalog.LookupModel("gpt-4o-2024-08-06")
	if !ok {
		t.Fatal("gpt-4o-2024-08-06 not found")
	}
	if m.InputPricePer1M != 2.5 {
		t.Errorf("gpt-4o input = %f, want 2.5", m.InputPricePer1M)
	}
	if m.ContextWindow != 128000 {
		t.Errorf("gpt-4o context = %d, want 128000", m.ContextWindow)
	}

	// gpt-4o has pricing but no context window → should be filtered out
	_, _, ok = catalog.LookupModel("gpt-4o")
	if ok {
		t.Error("gpt-4o should be filtered out (no context window)")
	}

	// Gemini model should have spec data from spec page
	gm, _, ok := catalog.LookupModel("gemini-3-flash-preview")
	if !ok {
		t.Fatal("gemini-3-flash-preview not found")
	}
	if gm.ContextWindow != 1048576 {
		t.Errorf("gemini context = %d, want 1048576", gm.ContextWindow)
	}
	if gm.MaxOutput != 65536 {
		t.Errorf("gemini maxOutput = %d, want 65536", gm.MaxOutput)
	}
}

func TestFetchLatest_FiltersIncomplete(t *testing.T) {
	// Models without context window or pricing should be filtered
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`### gpt-4o-2024-08-06

- Context window size: 128000
- Maximum output tokens: 16384

## Text tokens

| Name | Input | Cached input | Output | Unit |
| --- | --- | --- | --- | --- |
| gpt-4o-2024-08-06 | 2.5 | 1.25 | 10 | 1M tokens |
| gpt-4o | 2.5 | 1.25 | 10 | 1M tokens |
`))
	}))
	defer openAIServer.Close()

	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<table>
<tr><th>Feature</th><th>Model</th></tr>
<tr><td>Claude API ID</td><td>claude-sonnet-4-6</td></tr>
<tr><td>Pricing</td><td>$3 / input MTok $15 / output MTok</td></tr>
<tr><td>Context window</td><td>200K tokens</td></tr>
<tr><td>Max output</td><td>64K tokens</td></tr>
</table>`))
	}))
	defer anthropicServer.Close()

	geminiMux := http.NewServeMux()
	geminiMux.HandleFunc("/pricing", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<h3>Gemini 3 Flash Preview</h3><p>Input price $0.50</p><p>Output price $3.00</p>`))
	})
	geminiMux.HandleFunc("/models/gemini-3-flash-preview", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>Input token limit 1,048,576 Output token limit 65,536</html>`))
	})
	geminiServer := httptest.NewServer(geminiMux)
	defer geminiServer.Close()

	grokServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`\"name\":\"grok-3\",\"promptTextTokenPrice\":\"$n30000\",\"completionTextTokenPrice\":\"$n150000\",\"maxPromptLength\":131072`))
	}))
	defer grokServer.Close()

	origOpenAI, origAnthropic, origGemini, origGeminiSpec, origGrok := openAISourceURL, anthropicSourceURL, geminiSourceURL, geminiSpecBaseURL, grokSourceURL
	defer func() {
		openAISourceURL = origOpenAI
		anthropicSourceURL = origAnthropic
		geminiSourceURL = origGemini
		geminiSpecBaseURL = origGeminiSpec
		grokSourceURL = origGrok
	}()
	openAISourceURL = openAIServer.URL
	anthropicSourceURL = anthropicServer.URL
	geminiSourceURL = geminiServer.URL + "/pricing"
	geminiSpecBaseURL = geminiServer.URL + "/models"
	grokSourceURL = grokServer.URL

	catalog, err := FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest() error: %v", err)
	}

	// All models should have a context window. Pricing must be non-negative
	// but may be zero for free models (e.g. Gemma on Google AI Studio).
	for provider, models := range catalog.Providers {
		for _, m := range models {
			if m.InputPricePer1M < 0 || m.OutputPricePer1M < 0 {
				t.Errorf("%s/%s: negative pricing", provider, m.ID)
			}
			if m.ContextWindow <= 0 {
				t.Errorf("%s/%s: missing context window", provider, m.ID)
			}
		}
	}
}

func TestFetchLatest_ServerError(t *testing.T) {
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errorServer.Close()

	origOpenAI, origAnthropic, origGemini, origGrok := openAISourceURL, anthropicSourceURL, geminiSourceURL, grokSourceURL
	defer func() {
		openAISourceURL = origOpenAI
		anthropicSourceURL = origAnthropic
		geminiSourceURL = origGemini
		grokSourceURL = origGrok
	}()
	openAISourceURL = errorServer.URL
	anthropicSourceURL = errorServer.URL
	geminiSourceURL = errorServer.URL
	grokSourceURL = errorServer.URL

	_, err := FetchLatest(context.Background())
	if err == nil {
		t.Error("expected error when providers fail")
	}
}
