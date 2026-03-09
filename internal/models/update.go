package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
)

var fetchURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// setLiteLLMURL overrides the fetch URL (for testing).
func setLiteLLMURL(url string) { fetchURL = url }

// providerMap maps LiteLLM provider names to langdag provider names.
var providerMap = map[string]string{
	"anthropic": "anthropic",
	"openai":    "openai",
	"gemini":    "gemini",
	"xai":       "grok",
}

// litellmEntry represents a model entry in the LiteLLM JSON.
type litellmEntry struct {
	LiteLLMProvider    string  `json:"litellm_provider"`
	Mode               string  `json:"mode"`
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
	MaxInputTokens     int     `json:"max_input_tokens"`
	MaxOutputTokens    int     `json:"max_output_tokens"`
	MaxTokens          int     `json:"max_tokens"`
}

// FetchLatest fetches the latest model catalog from LiteLLM's GitHub repository.
func FetchLatest(ctx context.Context) (*Catalog, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("models: failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("models: failed to fetch model data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models: unexpected status %d from %s", resp.StatusCode, fetchURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("models: failed to read response: %w", err)
	}

	return parseLiteLLMData(body)
}

// parseLiteLLMData converts LiteLLM JSON data into a Catalog.
func parseLiteLLMData(data []byte) (*Catalog, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("models: failed to parse LiteLLM data: %w", err)
	}

	catalog := &Catalog{
		UpdatedAt: time.Now().UTC(),
		Source:    "litellm",
		Providers: make(map[string][]ModelPricing),
	}

	for key, rawEntry := range raw {
		var entry litellmEntry
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			continue
		}

		provider, ok := providerMap[entry.LiteLLMProvider]
		if !ok {
			continue
		}

		if entry.Mode != "chat" && entry.Mode != "responses" {
			continue
		}

		// Skip fine-tuned model templates
		if strings.HasPrefix(key, "ft:") {
			continue
		}

		// Strip provider prefix from model ID to get the actual API model ID
		modelID := key
		modelID = strings.TrimPrefix(modelID, "gemini/")
		modelID = strings.TrimPrefix(modelID, "xai/")

		contextWindow := entry.MaxInputTokens
		if contextWindow == 0 {
			contextWindow = entry.MaxTokens
		}

		pricing := ModelPricing{
			ID:               modelID,
			InputPricePer1M:  roundPrice(entry.InputCostPerToken * 1_000_000),
			OutputPricePer1M: roundPrice(entry.OutputCostPerToken * 1_000_000),
			ContextWindow:    contextWindow,
			MaxOutput:        entry.MaxOutputTokens,
		}

		catalog.Providers[provider] = append(catalog.Providers[provider], pricing)
	}

	// Sort models within each provider for deterministic output
	for provider := range catalog.Providers {
		slices.SortFunc(catalog.Providers[provider], func(a, b ModelPricing) int {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
	}

	return catalog, nil
}

// roundPrice rounds a price to 4 decimal places to avoid floating point artifacts.
func roundPrice(p float64) float64 {
	return math.Round(p*10000) / 10000
}
