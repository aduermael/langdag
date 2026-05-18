package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"langdag.com/langdag/internal/models"
	"langdag.com/langdag/types"
)

func TestDeploymentAwareRefreshEnablesNewCatalogModelIntegration(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "catalog.json")
	if err := models.SaveCatalog(models.ReferenceCatalogV1(), cachePath); err != nil {
		t.Fatalf("SaveCatalog old cache: %v", err)
	}

	remoteCatalog := models.ReferenceCatalogV1()
	remoteCatalog.GeneratedAt = time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	remoteCatalog.StaleAfter = remoteCatalog.GeneratedAt.Add(30 * 24 * time.Hour)
	remoteCatalog.Models["openai/gpt-phase8-refreshed"] = &models.ModelV1{
		ID:            "openai/gpt-phase8-refreshed",
		ProviderID:    "openai",
		Name:          "GPT Phase 8 Refreshed",
		ContextWindow: 128000,
	}
	remoteCatalog.Offerings = append(remoteCatalog.Offerings, models.ModelOfferingV1{
		ID:               "openai-direct:gpt-phase8-refreshed-2026-05-20",
		CanonicalModelID: "openai/gpt-phase8-refreshed",
		DeploymentID:     "openai-direct",
		NativeModelID:    "gpt-phase8-refreshed-2026-05-20",
		Pricing: models.PricingV1{
			Status:      models.PricingKnown,
			Currency:    "USD",
			EffectiveAt: remoteCatalog.GeneratedAt,
			RatesPer1M:  map[string]float64{"input_tokens": 2, "output_tokens": 8},
		},
	})
	server := serveProviderCatalog(t, remoteCatalog)
	defer server.Close()

	refresh, err := models.RefreshCatalogCache(context.Background(), models.CatalogRefreshOptions{
		CachePath: cachePath,
		Endpoint:  server.URL,
		Timeout:   time.Second,
		Now:       func() time.Time { return remoteCatalog.GeneratedAt.Add(time.Hour) },
	})
	if err != nil {
		t.Fatalf("RefreshCatalogCache: %v", err)
	}
	if !refresh.ReplacedCache || refresh.Catalog == nil {
		t.Fatalf("refresh result = %+v, want cache replacement", refresh)
	}

	loaded, err := models.LoadCatalogWithOptions(models.CatalogLoadOptions{CachePath: cachePath})
	if err != nil {
		t.Fatalf("LoadCatalogWithOptions: %v", err)
	}
	if loaded.Source != models.CatalogSourceCache {
		t.Fatalf("loaded source = %q, want cache", loaded.Source)
	}
	compiled, err := models.CompileCatalogV1(loaded.Catalog)
	if err != nil {
		t.Fatalf("CompileCatalogV1: %v", err)
	}

	openAI := &captureProvider{name: "openai-direct"}
	router, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: compiled,
		Deployments: map[string]DeploymentAdapter{
			"openai-direct": deploymentAdapter("openai-direct", openAI),
		},
	})
	if err != nil {
		t.Fatalf("NewDeploymentRouter: %v", err)
	}
	resp, err := router.Complete(context.Background(), &types.CompletionRequest{
		Model:    "openai/gpt-phase8-refreshed",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}})
	if err != nil {
		t.Fatalf("Complete refreshed model: %v", err)
	}
	if openAI.lastReq.Model != "gpt-phase8-refreshed-2026-05-20" {
		t.Fatalf("native model = %q, want refreshed catalog native id", openAI.lastReq.Model)
	}
	if resp.ModelResolution == nil || resp.ModelResolution.OfferingID != "openai-direct:gpt-phase8-refreshed-2026-05-20" {
		t.Fatalf("model resolution = %+v", resp.ModelResolution)
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if !json.Valid(data) {
		t.Fatal("refreshed cache is not valid JSON")
	}
}

func serveProviderCatalog(t *testing.T, catalog *models.Catalog) *httptest.Server {
	t.Helper()
	if err := models.ValidateCatalogV1(catalog); err != nil {
		t.Fatalf("test catalog is invalid: %v", err)
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
}
