package types

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestNormalizedUsagePreservesLegacyAndExtensibleDimensions(t *testing.T) {
	usage := NormalizedUsageFromUsage(Usage{
		InputTokens:              1000,
		OutputTokens:             500,
		CacheReadInputTokens:     250,
		CacheCreationInputTokens: 100,
		ReasoningTokens:          75,
		ToolUsePromptTokens:      20,
		AudioInputTokens:         12,
		Dimensions:               map[string]int64{"provider_custom_units": 4},
	})
	usage.Dimensions = map[string]int64{"image_input_tokens": 12, "input_tokens": 8}

	dimensions := usage.BillableDimensions()
	if dimensions["input_tokens"] != 1008 {
		t.Fatalf("input_tokens = %d, want common and extensible dimensions aggregated", dimensions["input_tokens"])
	}
	for _, name := range []string{"output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens", "reasoning_tokens", "tool_use_prompt_tokens", "audio_input_tokens", "image_input_tokens"} {
		if dimensions[name] == 0 {
			t.Errorf("dimension %q missing from billable dimensions: %+v", name, dimensions)
		}
	}
}

func TestComputeCostFromPricingSnapshotStatuses(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	usage := NormalizedUsage{
		InputTokens:          1000,
		OutputTokens:         500,
		ReasoningTokens:      100,
		CacheReadInputTokens: 200,
	}
	snapshot := PricingSnapshot{
		Status:      CostStatusKnown,
		Currency:    "USD",
		EffectiveAt: effectiveAt,
		Source:      CostSourceCatalog,
		RatesPer1M: map[string]float64{
			"input_tokens":            2,
			"output_tokens":           8,
			"cache_read_input_tokens": 0.5,
		},
	}

	result := ComputeCostFromPricingSnapshot(snapshot, usage)
	if result.Status != CostStatusPartial {
		t.Fatalf("Status = %q, want partial because reasoning_tokens are missing", result.Status)
	}
	want := (1000*2 + 500*8 + 200*0.5) / 1_000_000
	if math.Abs(result.Total-want) > 1e-12 {
		t.Fatalf("Total = %.12f, want %.12f", result.Total, want)
	}
	if len(result.MissingDimensions) != 1 || result.MissingDimensions[0] != "reasoning_tokens" {
		t.Fatalf("MissingDimensions = %+v, want reasoning_tokens", result.MissingDimensions)
	}

	unknown := ComputeCostFromPricingSnapshot(PricingSnapshot{Status: CostStatusUnknown, Currency: "USD", Source: CostSourceCatalog}, usage)
	if unknown.Status != CostStatusUnknown || unknown.Total != 0 {
		t.Fatalf("unknown result = %+v", unknown)
	}

	free := ComputeCostFromPricingSnapshot(PricingSnapshot{Status: CostStatusFree, Currency: "USD", Source: CostSourceCatalog}, usage)
	if free.Status != CostStatusFree || free.Total != 0 {
		t.Fatalf("free result = %+v", free)
	}

	future := ComputeCostFromPricingSnapshot(PricingSnapshot{Status: CostStatusKnown, Currency: "USD", Source: CostSourceCatalog, EffectiveAt: time.Now().Add(time.Hour), RatesPer1M: map[string]float64{"input_tokens": 1}}, usage)
	if future.Status != CostStatusUnknown || len(future.MissingDimensions) != 1 || future.MissingDimensions[0] != "pricing_effective_at" {
		t.Fatalf("future effective pricing result = %+v", future)
	}

	exact := ComputeCost(&ProviderCost{Total: 0.123, Currency: "USD", Source: CostSourceProviderResponse}, &snapshot, usage)
	if exact.Source != CostSourceProviderResponse || exact.Total != 0.123 {
		t.Fatalf("provider exact cost should take precedence: %+v", exact)
	}
}

func TestAssistantNodeMetadataRoundTrips(t *testing.T) {
	meta := AssistantNodeMetadata{
		ModelResolution: &ModelResolutionMetadata{
			CanonicalModelID: "openai/gpt-4.1-2025-04-14",
			OfferingID:       "openai-direct:gpt-4.1-2025-04-14",
			DeploymentID:     "openai-direct",
			ProviderID:       "openai",
			APIProtocolID:    "openai-chat-completions",
			NativeModelID:    "gpt-4.1-2025-04-14",
		},
		NormalizedUsage: &NormalizedUsage{InputTokens: 10, OutputTokens: 20},
		PricingSnapshot: &PricingSnapshot{
			Status:     CostStatusKnown,
			Currency:   "USD",
			Source:     CostSourceCatalog,
			RatesPer1M: map[string]float64{"input_tokens": 2, "output_tokens": 8},
		},
		ProviderCost: &ProviderCost{Total: 0.001, Currency: "USD", Source: CostSourceProviderResponse},
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundTrip AssistantNodeMetadata
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if roundTrip.ModelResolution == nil || roundTrip.ModelResolution.DeploymentID != "openai-direct" {
		t.Fatalf("roundTrip.ModelResolution = %+v", roundTrip.ModelResolution)
	}
	if roundTrip.ProviderCost == nil || roundTrip.ProviderCost.Source != CostSourceProviderResponse {
		t.Fatalf("roundTrip.ProviderCost = %+v", roundTrip.ProviderCost)
	}
}
