package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"langdag.com/langdag/internal/models"
	"langdag.com/langdag/internal/provider"
	"langdag.com/langdag/internal/storage/sqlite"
	"langdag.com/langdag/types"
)

type rolloutProvider struct {
	name         string
	failCount    int
	calls        int
	lastReq      *types.CompletionRequest
	providerCost *types.ProviderCost
}

func (p *rolloutProvider) Name() string { return p.name }

func (p *rolloutProvider) Models() []types.ModelInfo { return nil }

func (p *rolloutProvider) Complete(_ context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	p.calls++
	copied := *req
	p.lastReq = &copied
	if p.failCount > 0 {
		p.failCount--
		return nil, fmt.Errorf("%s unavailable", p.name)
	}
	return rolloutResponse(p.name, req.Model, p.providerCost), nil
}

func (p *rolloutProvider) Stream(_ context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	p.calls++
	copied := *req
	p.lastReq = &copied
	if p.failCount > 0 {
		p.failCount--
		return nil, fmt.Errorf("%s unavailable", p.name)
	}
	ch := make(chan types.StreamEvent, 3)
	ch <- types.StreamEvent{Type: types.StreamEventStart}
	ch <- types.StreamEvent{Type: types.StreamEventDelta, Content: "served by " + p.name}
	ch <- types.StreamEvent{Type: types.StreamEventDone, Response: rolloutResponse(p.name, req.Model, p.providerCost)}
	close(ch)
	return ch, nil
}

func rolloutResponse(providerName, model string, providerCost *types.ProviderCost) *types.CompletionResponse {
	return &types.CompletionResponse{
		ID:         "rollout-" + providerName,
		Model:      model,
		Provider:   providerName,
		Content:    []types.ContentBlock{{Type: "text", Text: "served by " + providerName}},
		StopReason: "end_turn",
		Usage: types.Usage{
			InputTokens:           100,
			OutputTokens:          40,
			CacheWriteInputTokens: 10,
			ToolUsePromptTokens:   5,
			Dimensions:            map[string]int64{"provider_cached_audio_tokens": 7},
		},
		ProviderCost: providerCost,
	}
}

func TestDeploymentAwareConversationPersistsRoutedAccountingIntegration(t *testing.T) {
	catalog := models.ReferenceCatalogV1()
	catalog.Models["openai/gpt-phase8-integration"] = &models.ModelV1{
		ID:            "openai/gpt-phase8-integration",
		ProviderID:    "openai",
		Name:          "GPT Phase 8 Integration",
		ContextWindow: 128000,
	}
	generatedAt := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	catalog.GeneratedAt = generatedAt
	catalog.StaleAfter = generatedAt.Add(30 * 24 * time.Hour)
	catalog.Offerings = append(catalog.Offerings,
		models.ModelOfferingV1{
			ID:               "openai-direct:gpt-phase8-integration",
			CanonicalModelID: "openai/gpt-phase8-integration",
			DeploymentID:     "openai-direct",
			NativeModelID:    "gpt-phase8-integration",
			Pricing: models.PricingV1{
				Status:      models.PricingKnown,
				Currency:    "USD",
				EffectiveAt: generatedAt,
				RatesPer1M:  map[string]float64{"input_tokens": 2, "output_tokens": 8},
			},
		},
		models.ModelOfferingV1{
			ID:               "openrouter:openai/gpt-phase8-integration-native",
			CanonicalModelID: "openai/gpt-phase8-integration",
			DeploymentID:     "openrouter",
			NativeModelID:    "openai/gpt-phase8-integration-native",
			Pricing: models.PricingV1{
				Status:      models.PricingKnown,
				Currency:    "USD",
				EffectiveAt: generatedAt,
				RatesPer1M:  map[string]float64{"input_tokens": 3, "output_tokens": 15, "cache_write_input_tokens": 1},
			},
		},
	)
	compiled, err := models.CompileCatalogV1(catalog)
	if err != nil {
		t.Fatalf("CompileCatalogV1: %v", err)
	}

	primary := &rolloutProvider{name: "openai-direct", failCount: 2}
	fallback := &rolloutProvider{
		name: "openrouter",
		providerCost: &types.ProviderCost{
			Total:    0.0123,
			Currency: "USD",
			Source:   types.CostSourceProviderResponse,
			Raw:      json.RawMessage(`{"credits":0.0123}`),
		},
	}
	router, err := provider.NewDeploymentRouter(provider.DeploymentRouterOptions{
		Catalog: compiled,
		Deployments: map[string]provider.DeploymentAdapter{
			"openai-direct": {DeploymentID: "openai-direct", Provider: primary},
			"openrouter":    {DeploymentID: "openrouter", Provider: fallback},
		},
		Routing: provider.RoutingPolicy{Default: []provider.RoutingStage{
			{Deployments: []provider.DeploymentChoice{{DeploymentID: "openai-direct", Weight: 100}}, Retries: 1},
			{Deployments: []provider.DeploymentChoice{{DeploymentID: "openrouter", Weight: 100}}},
		}},
	})
	if err != nil {
		t.Fatalf("NewDeploymentRouter: %v", err)
	}

	store, err := sqlite.New(filepath.Join(t.TempDir(), "conversation.db"))
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	defer store.Close()
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	mgr := NewManager(store, router)

	events, err := mgr.Prompt(context.Background(), "route this", "openai/gpt-phase8-integration", "", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	var nodeID string
	for _, event := range drainEvents(t, events, 5*time.Second) {
		if event.Type == types.StreamEventNodeSaved {
			nodeID = event.NodeID
		}
	}
	if primary.calls != 2 {
		t.Fatalf("primary calls = %d, want one try plus one retry", primary.calls)
	}
	if primary.lastReq == nil || primary.lastReq.Model != "gpt-phase8-integration" {
		t.Fatalf("primary request model = %+v, want direct native id", primary.lastReq)
	}
	if fallback.calls != 1 || fallback.lastReq == nil || fallback.lastReq.Model != "openai/gpt-phase8-integration-native" {
		t.Fatalf("fallback calls/request = %d/%+v", fallback.calls, fallback.lastReq)
	}
	if nodeID == "" {
		t.Fatal("missing saved assistant node")
	}

	node, err := store.GetNode(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	metadata, err := types.ParseAssistantNodeMetadata(node.Metadata)
	if err != nil {
		t.Fatalf("ParseAssistantNodeMetadata: %v", err)
	}
	if metadata == nil || metadata.ModelResolution == nil {
		t.Fatalf("missing model resolution metadata: %s", string(node.Metadata))
	}
	if metadata.ModelResolution.DeploymentID != "openrouter" ||
		metadata.ModelResolution.CanonicalModelID != "openai/gpt-phase8-integration" ||
		metadata.ModelResolution.NativeModelID != "openai/gpt-phase8-integration-native" ||
		metadata.ModelResolution.OfferingID != "openrouter:openai/gpt-phase8-integration-native" ||
		metadata.ModelResolution.ProviderID != "openai" ||
		metadata.ModelResolution.APIProtocolID != "openai-chat-completions" {
		t.Fatalf("bad model resolution: %+v", metadata.ModelResolution)
	}
	if metadata.PricingSnapshot == nil || metadata.PricingSnapshot.RatesPer1M["cache_write_input_tokens"] != 1 {
		t.Fatalf("pricing snapshot did not persist routed offering pricing: %+v", metadata.PricingSnapshot)
	}
	if metadata.NormalizedUsage == nil ||
		metadata.NormalizedUsage.CacheWriteInputTokens != 10 ||
		metadata.NormalizedUsage.ToolUsePromptTokens != 5 ||
		metadata.NormalizedUsage.Dimensions["provider_cached_audio_tokens"] != 7 {
		t.Fatalf("normalized usage did not preserve returned dimensions: %+v", metadata.NormalizedUsage)
	}
	if metadata.ProviderCost == nil || metadata.ProviderCost.Total != 0.0123 {
		t.Fatalf("provider exact cost did not persist: %+v", metadata.ProviderCost)
	}
	cost := types.ComputeCost(metadata.ProviderCost, metadata.PricingSnapshot, *metadata.NormalizedUsage)
	if cost.Source != types.CostSourceProviderResponse || cost.Total != 0.0123 {
		t.Fatalf("provider exact cost should be authoritative, got %+v", cost)
	}
}
