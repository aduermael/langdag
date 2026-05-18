package langdag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"langdag.com/langdag/types"
)

type phase8OpenAICompatServer struct {
	server      *httptest.Server
	mu          sync.Mutex
	status      int
	contentType string
	calls       int
	models      []string
}

func newPhase8OpenAICompatServer(t *testing.T, status int, body string) *phase8OpenAICompatServer {
	t.Helper()
	s := &phase8OpenAICompatServer{status: status, contentType: "application/json"}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		s.mu.Lock()
		s.calls++
		s.models = append(s.models, req.Model)
		s.mu.Unlock()
		if status != http.StatusOK {
			http.Error(w, "temporary upstream failure", status)
			return
		}
		w.Header().Set("Content-Type", s.contentType)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *phase8OpenAICompatServer) URL() string {
	return s.server.URL
}

func (s *phase8OpenAICompatServer) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *phase8OpenAICompatServer) Models() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.models...)
}

func TestNewLoadsRuntimeCatalogCacheForRouting(t *testing.T) {
	const canonicalID = "openai/gpt-runtime-cache-public"
	const nativeID = "gpt-runtime-cache-public-native"

	home := t.TempDir()
	t.Setenv("HOME", home)

	catalog := ReferenceCatalogV1()
	generatedAt := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	catalog.GeneratedAt = generatedAt
	catalog.StaleAfter = generatedAt.Add(30 * 24 * time.Hour)
	catalog.Models[canonicalID] = &ModelV1{
		ID:            canonicalID,
		ProviderID:    "openai",
		Name:          "GPT Runtime Cache Public",
		ContextWindow: 128000,
	}
	catalog.Offerings = append(catalog.Offerings, ModelOfferingV1{
		ID:               "openai-direct:" + nativeID,
		CanonicalModelID: canonicalID,
		DeploymentID:     "openai-direct",
		NativeModelID:    nativeID,
		Pricing: PricingV1{
			Status:      PricingKnown,
			Currency:    "USD",
			EffectiveAt: generatedAt,
			RatesPer1M:  map[string]float64{"input_tokens": 2, "output_tokens": 8},
		},
	})
	writeCatalogCache(t, DefaultModelCatalogCachePath(), catalog)

	body := `data: {"id":"chatcmpl-runtime-cache","model":"` + nativeID + `","choices":[{"index":0,"delta":{"content":"cache route"},"finish_reason":null}]}

data: {"id":"chatcmpl-runtime-cache","model":"` + nativeID + `","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2}}

data: {"id":"chatcmpl-runtime-cache","model":"` + nativeID + `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	server := newPhase8OpenAICompatServer(t, http.StatusOK, body)
	server.contentType = "text/event-stream"

	client, err := New(Config{
		StoragePath: filepath.Join(t.TempDir(), "runtime-cache.db"),
		Provider:    "openai",
		APIKeys:     map[string]string{"openai": "sk-test"},
		OpenAIConfig: &OpenAIConfig{
			BaseURL: server.URL(),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer client.Close()

	result, err := client.Prompt(context.Background(), "use the runtime cache", WithModel(canonicalID))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	var final StreamChunk
	for chunk := range result.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.Done {
			final = chunk
		}
	}
	if got := server.Models(); len(got) != 1 || got[0] != nativeID {
		t.Fatalf("request models = %+v, want native id %q from runtime cache", got, nativeID)
	}
	if final.ModelResolution == nil || final.ModelResolution.NativeModelID != nativeID {
		t.Fatalf("model resolution = %+v, want native id %q", final.ModelResolution, nativeID)
	}
}

func TestDeploymentAwarePublicClientRoutingAndMetadataIntegration(t *testing.T) {
	const canonicalID = "openai/gpt-phase8-public"
	const directNativeID = "gpt-phase8-public-direct"
	const openRouterNativeID = "openai/gpt-phase8-public-native"

	catalog := ReferenceCatalogV1()
	generatedAt := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	catalog.GeneratedAt = generatedAt
	catalog.StaleAfter = generatedAt.Add(30 * 24 * time.Hour)
	catalog.Models[canonicalID] = &ModelV1{
		ID:            canonicalID,
		ProviderID:    "openai",
		Name:          "GPT Phase 8 Public",
		ContextWindow: 128000,
	}
	catalog.Offerings = append(catalog.Offerings,
		ModelOfferingV1{
			ID:               "openai-direct:" + directNativeID,
			CanonicalModelID: canonicalID,
			DeploymentID:     "openai-direct",
			NativeModelID:    directNativeID,
			Pricing: PricingV1{
				Status:      PricingKnown,
				Currency:    "USD",
				EffectiveAt: generatedAt,
				RatesPer1M:  map[string]float64{"input_tokens": 2, "output_tokens": 8},
			},
		},
		ModelOfferingV1{
			ID:               "openrouter:" + openRouterNativeID,
			CanonicalModelID: canonicalID,
			DeploymentID:     "openrouter",
			NativeModelID:    openRouterNativeID,
			Pricing: PricingV1{
				Status:      PricingKnown,
				Currency:    "USD",
				EffectiveAt: generatedAt,
				RatesPer1M:  map[string]float64{"input_tokens": 3, "output_tokens": 15, "reasoning_tokens": 1},
			},
		},
	)

	primary := newPhase8OpenAICompatServer(t, http.StatusBadGateway, "")
	fallbackBody := `data: {"id":"chatcmpl-public","model":"` + openRouterNativeID + `","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-public","model":"` + openRouterNativeID + `","choices":[{"index":0,"delta":{"content":"public "},"finish_reason":null}]}

data: {"id":"chatcmpl-public","model":"` + openRouterNativeID + `","choices":[{"index":0,"delta":{"content":"route"},"finish_reason":null}]}

data: {"id":"chatcmpl-public","model":"` + openRouterNativeID + `","choices":[],"usage":{"prompt_tokens":125,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":25},"completion_tokens_details":{"reasoning_tokens":3},"cost":0.045}}

data: {"id":"chatcmpl-public","model":"` + openRouterNativeID + `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	fallback := newPhase8OpenAICompatServer(t, http.StatusOK, fallbackBody)
	fallback.contentType = "text/event-stream"

	client, err := New(Config{
		StoragePath:  filepath.Join(t.TempDir(), "public-integration.db"),
		ModelCatalog: catalog,
		Deployments: map[string]DeploymentConfig{
			"openai-direct": {APIKey: "sk-direct", BaseURL: primary.URL()},
			"openrouter":    {APIKey: "sk-or", BaseURL: fallback.URL()},
		},
		RoutingPolicy: &RoutingPolicy{Default: []RoutingStage{
			{Deployments: []DeploymentChoice{{DeploymentID: "openai-direct", Weight: 100}}, Retries: 1},
			{Deployments: []DeploymentChoice{{DeploymentID: "openrouter", Weight: 100}}},
		}},
		RetryConfig: &RetryConfig{MaxRetries: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer client.Close()

	result, err := client.Prompt(context.Background(), "use the public deployment router", WithModel(canonicalID))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	var final StreamChunk
	for chunk := range result.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.Done {
			final = chunk
		}
	}
	if final.NodeID == "" {
		t.Fatal("missing saved node id from PromptResult stream")
	}
	if result.GetContent() != "public route" {
		t.Fatalf("stream content = %q", result.GetContent())
	}
	if primary.Calls() < 2 {
		t.Fatalf("primary calls = %d, want retry before fallback", primary.Calls())
	}
	for _, model := range primary.Models() {
		if model != directNativeID {
			t.Fatalf("primary request model = %q, want direct native id %q", model, directNativeID)
		}
	}
	if fallback.Calls() != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.Calls())
	}
	if got := fallback.Models(); len(got) != 1 || got[0] != openRouterNativeID {
		t.Fatalf("fallback request models = %+v, want native id %q", got, openRouterNativeID)
	}
	if final.ModelResolution == nil ||
		final.ModelResolution.CanonicalModelID != canonicalID ||
		final.ModelResolution.DeploymentID != "openrouter" ||
		final.ModelResolution.NativeModelID != openRouterNativeID ||
		final.ModelResolution.OfferingID != "openrouter:"+openRouterNativeID {
		t.Fatalf("final model resolution = %+v", final.ModelResolution)
	}
	if final.PricingSnapshot == nil || final.PricingSnapshot.RatesPer1M["reasoning_tokens"] != 1 {
		t.Fatalf("final pricing snapshot = %+v", final.PricingSnapshot)
	}
	if final.ProviderCost == nil ||
		final.ProviderCost.Source != types.CostSourceProviderResponse ||
		final.ProviderCost.Total != 0.045 {
		t.Fatalf("final provider cost = %+v", final.ProviderCost)
	}
	if final.NormalizedUsage == nil ||
		final.NormalizedUsage.InputTokens != 100 ||
		final.NormalizedUsage.CacheReadInputTokens != 25 ||
		final.NormalizedUsage.ReasoningTokens != 3 {
		t.Fatalf("final normalized usage = %+v", final.NormalizedUsage)
	}

	node, err := client.GetNode(context.Background(), final.NodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	metadata, err := types.ParseAssistantNodeMetadata(node.Metadata)
	if err != nil {
		t.Fatalf("ParseAssistantNodeMetadata: %v", err)
	}
	if metadata == nil ||
		metadata.ModelResolution == nil ||
		metadata.ModelResolution.NativeModelID != openRouterNativeID ||
		metadata.ProviderCost == nil ||
		metadata.ProviderCost.Total != 0.045 {
		t.Fatalf("stored metadata = %+v", metadata)
	}
}

func writeCatalogCache(t *testing.T, path string, catalog *ModelCatalog) {
	t.Helper()
	if path == "" {
		t.Fatal("catalog cache path is empty")
	}
	if err := ValidateCatalogV1(catalog); err != nil {
		t.Fatalf("catalog is invalid: %v", err)
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create catalog cache dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write catalog cache: %v", err)
	}
}
