package models

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
)

// providerServerTools maps provider names to the server tools their models support.
// Used to annotate fetched catalog entries with capability metadata.
var providerServerTools = map[string][]string{
	"anthropic": {"web_search"},
	"openai":    {"web_search"},
	"gemini":    {"web_search"},
	"grok":      {"web_search"},
}

// FetchLatest fetches the latest model catalog from official provider documentation pages.
// All provider fetches must succeed; partial failures return an error.
func FetchLatest(ctx context.Context) (*Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	type result struct {
		provider string
		models   []ModelPricing
		err      error
	}

	type fetcher struct {
		provider string
		fn       func(context.Context) ([]ModelPricing, error)
	}

	fetchers := []fetcher{
		{"openai", fetchOpenAIModels},
		{"anthropic", fetchAnthropicModels},
		{"gemini", fetchGeminiAndGemmaModels},
		{"grok", fetchGrokModels},
	}

	results := make([]result, len(fetchers))
	var wg sync.WaitGroup
	for i, f := range fetchers {
		wg.Add(1)
		go func(i int, f fetcher) {
			defer wg.Done()
			models, err := f.fn(ctx)
			results[i] = result{provider: f.provider, models: models, err: err}
		}(i, f)
	}
	wg.Wait()

	// All providers must succeed
	var errs []string
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.provider, r.err))
		} else if len(r.models) == 0 {
			errs = append(errs, fmt.Sprintf("%s: no models found", r.provider))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("models: provider fetch failed: %s", strings.Join(errs, "; "))
	}

	catalog := &Catalog{
		UpdatedAt: time.Now().UTC(),
		Source:    "providers",
		Providers: make(map[string][]ModelPricing),
	}

	for _, r := range results {
		// Filter: require pricing > 0 AND context_window > 0
		var filtered []ModelPricing
		for _, m := range r.models {
			if (m.InputPricePer1M > 0 || m.OutputPricePer1M > 0) && m.ContextWindow > 0 {
				filtered = append(filtered, m)
			}
		}
		// Annotate with known server tool capabilities.
		if st := providerServerTools[r.provider]; len(st) > 0 {
			for i := range filtered {
				filtered[i].ServerTools = st
			}
		}
		slices.SortFunc(filtered, func(a, b ModelPricing) int {
			return strings.Compare(a.ID, b.ID)
		})
		catalog.Providers[r.provider] = filtered
	}

	return catalog, nil
}

// roundPrice rounds a price to 4 decimal places to avoid floating point artifacts.
func roundPrice(p float64) float64 {
	return math.Round(p*10000) / 10000
}
