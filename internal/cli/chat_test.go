package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"langdag.com/langdag"
	"langdag.com/langdag/internal/config"
	"langdag.com/langdag/internal/models"
)

func TestConvertRoutingStagesPreservesExplicitEmptyDefault(t *testing.T) {
	stages := convertRoutingStages([]config.RoutingStage{})
	if stages == nil || len(stages) != 0 {
		t.Fatalf("empty route was not preserved: %+v", stages)
	}

	stages = convertRoutingStages(nil)
	if stages != nil {
		t.Fatalf("nil route should remain nil: %+v", stages)
	}

	stageMap := convertRoutingStageMap(map[string][]config.RoutingStage{"openai": {}})
	stages, ok := stageMap["openai"]
	if !ok || stages == nil || len(stages) != 0 {
		t.Fatalf("empty provider route was not preserved: %+v", stageMap)
	}
}

func TestNewLibraryClientUsesRuntimeCatalogCache(t *testing.T) {
	const canonicalID = "openai/gpt-runtime-cache-cli"
	const nativeID = "gpt-runtime-cache-cli-native"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LANGDAG_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("LANGDAG_STORAGE_PATH", filepath.Join(t.TempDir(), "cli.db"))

	catalog := models.ReferenceCatalogV1()
	generatedAt := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	catalog.GeneratedAt = generatedAt
	catalog.StaleAfter = generatedAt.Add(30 * 24 * time.Hour)
	catalog.Models[canonicalID] = &models.ModelV1{
		ID:            canonicalID,
		ProviderID:    "openai",
		Name:          "GPT Runtime Cache CLI",
		ContextWindow: 128000,
	}
	catalog.Offerings = append(catalog.Offerings, models.ModelOfferingV1{
		ID:               "openai-direct:" + nativeID,
		CanonicalModelID: canonicalID,
		DeploymentID:     "openai-direct",
		NativeModelID:    nativeID,
		Pricing: models.PricingV1{
			Status:      models.PricingKnown,
			Currency:    "USD",
			EffectiveAt: generatedAt,
			RatesPer1M:  map[string]float64{"input_tokens": 2, "output_tokens": 8},
		},
	})
	if err := models.SaveCatalog(catalog, models.DefaultCatalogCachePath()); err != nil {
		t.Fatalf("SaveCatalog: %v", err)
	}

	var requestedModel string
	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestedModel = req.Model
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, `data: {"id":"chatcmpl-cache-cli","model":%q,"choices":[{"index":0,"delta":{"content":"cache cli"},"finish_reason":null}]}

data: {"id":"chatcmpl-cache-cli","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}

data: [DONE]

`, nativeID, nativeID)
	}))
	defer openAI.Close()
	t.Setenv("OPENAI_BASE_URL", openAI.URL)

	client, err := newLibraryClient(context.Background())
	if err != nil {
		t.Fatalf("newLibraryClient: %v", err)
	}
	defer client.Close()

	result, err := client.Prompt(context.Background(), "use cli runtime cache", langdag.WithModel(canonicalID))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	for chunk := range result.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
	}
	if requestedModel != nativeID {
		t.Fatalf("request model = %q, want runtime cache native id %q", requestedModel, nativeID)
	}
}
