package models

import (
	"testing"
)

func TestParseOpenAIText(t *testing.T) {
	text := `# GPT-4o

**Current Snapshot:** gpt-4o-2024-08-06

## Snapshots

### gpt-4o-2024-08-06

- Context window size: 128000
- Knowledge cutoff date: 2023-10-01
- Maximum output tokens: 16384

### gpt-4o-mini-2024-07-18

- Context window size: 128000
- Maximum output tokens: 16384

## Text tokens

| Name | Input | Cached input | Output | Unit |
| --- | --- | --- | --- | --- |
| gpt-4o | 2.5 | 1.25 | 10 | 1M tokens |
| gpt-4o (batch) | 1.25 | | 5 | 1M tokens |
| gpt-4o-2024-08-06 | 2.5 | 1.25 | 10 | 1M tokens |
| gpt-4o-mini | 0.15 | 0.075 | 0.6 | 1M tokens |
| o1-pro | 100 | | 400 | 1M tokens |

## Audio tokens
`

	models, err := parseOpenAIText(text)
	if err != nil {
		t.Fatalf("parseOpenAIText error: %v", err)
	}

	byID := make(map[string]ModelPricing)
	for _, m := range models {
		byID[m.ID] = m
	}

	// gpt-4o: pricing only (no metadata section for family name)
	if m, ok := byID["gpt-4o"]; !ok {
		t.Error("gpt-4o not found")
	} else {
		if m.InputPricePer1M != 2.5 {
			t.Errorf("gpt-4o input = %f, want 2.5", m.InputPricePer1M)
		}
		if m.OutputPricePer1M != 10 {
			t.Errorf("gpt-4o output = %f, want 10", m.OutputPricePer1M)
		}
	}

	// gpt-4o-2024-08-06: pricing + metadata
	if m, ok := byID["gpt-4o-2024-08-06"]; !ok {
		t.Error("gpt-4o-2024-08-06 not found")
	} else {
		if m.InputPricePer1M != 2.5 {
			t.Errorf("gpt-4o-2024-08-06 input = %f, want 2.5", m.InputPricePer1M)
		}
		if m.ContextWindow != 128000 {
			t.Errorf("gpt-4o-2024-08-06 context = %d, want 128000", m.ContextWindow)
		}
		if m.MaxOutput != 16384 {
			t.Errorf("gpt-4o-2024-08-06 maxOutput = %d, want 16384", m.MaxOutput)
		}
	}

	// gpt-4o-mini-2024-07-18: metadata only (no pricing row for this snapshot)
	if m, ok := byID["gpt-4o-mini-2024-07-18"]; !ok {
		t.Error("gpt-4o-mini-2024-07-18 not found")
	} else {
		if m.ContextWindow != 128000 {
			t.Errorf("context = %d, want 128000", m.ContextWindow)
		}
		if m.InputPricePer1M != 0 {
			t.Errorf("input = %f, want 0 (no pricing row)", m.InputPricePer1M)
		}
	}

	// Batch entries should be filtered
	if _, ok := byID["gpt-4o (batch)"]; ok {
		t.Error("batch entry should be filtered out")
	}

	// o1-pro: high pricing
	if m, ok := byID["o1-pro"]; !ok {
		t.Error("o1-pro not found")
	} else {
		if m.InputPricePer1M != 100 {
			t.Errorf("o1-pro input = %f, want 100", m.InputPricePer1M)
		}
		if m.OutputPricePer1M != 400 {
			t.Errorf("o1-pro output = %f, want 400", m.OutputPricePer1M)
		}
	}
}

func TestParseAnthropicHTML(t *testing.T) {
	html := `<html><body>
<table>
<tr><th>Feature</th><th>Claude Opus 4.6</th><th>Claude Sonnet 4.6</th></tr>
<tr><td><strong>Claude API ID</strong></td><td>claude-opus-4-6</td><td>claude-sonnet-4-6</td></tr>
<tr><td><strong>Claude API alias</strong></td><td>claude-opus-4-6</td><td>claude-sonnet-4-6</td></tr>
<tr><td><strong>Pricing</strong></td><td>$5 / input MTok<br/>$25 / output MTok</td><td>$3 / input MTok<br/>$15 / output MTok</td></tr>
<tr><td><strong>Context window</strong></td><td>200K tokens</td><td>200K tokens</td></tr>
<tr><td><strong>Max output</strong></td><td>128K tokens</td><td>64K tokens</td></tr>
</table>
<table>
<tr><th>Feature</th><th>Claude Sonnet 4.5</th></tr>
<tr><td><strong>Claude API ID</strong></td><td>claude-sonnet-4-5-20250929</td></tr>
<tr><td><strong>Claude API alias</strong></td><td>claude-sonnet-4-5</td></tr>
<tr><td><strong>Pricing</strong></td><td>$3 / input MTok<br/>$15 / output MTok</td></tr>
<tr><td><strong>Context window</strong></td><td>200K tokens</td></tr>
<tr><td><strong>Max output</strong></td><td>64K tokens</td></tr>
</table>
</body></html>`

	models, err := parseAnthropicHTML(html)
	if err != nil {
		t.Fatalf("parseAnthropicHTML error: %v", err)
	}

	byID := make(map[string]ModelPricing)
	for _, m := range models {
		byID[m.ID] = m
	}

	// Opus 4.6
	if m, ok := byID["claude-opus-4-6"]; !ok {
		t.Error("claude-opus-4-6 not found")
	} else {
		if m.InputPricePer1M != 5 {
			t.Errorf("opus input = %f, want 5", m.InputPricePer1M)
		}
		if m.OutputPricePer1M != 25 {
			t.Errorf("opus output = %f, want 25", m.OutputPricePer1M)
		}
		if m.ContextWindow != 200000 {
			t.Errorf("opus context = %d, want 200000", m.ContextWindow)
		}
		if m.MaxOutput != 128000 {
			t.Errorf("opus maxOutput = %d, want 128000", m.MaxOutput)
		}
	}

	// Sonnet 4.6
	if m, ok := byID["claude-sonnet-4-6"]; !ok {
		t.Error("claude-sonnet-4-6 not found")
	} else {
		if m.OutputPricePer1M != 15 {
			t.Errorf("sonnet output = %f, want 15", m.OutputPricePer1M)
		}
		if m.MaxOutput != 64000 {
			t.Errorf("sonnet maxOutput = %d, want 64000", m.MaxOutput)
		}
	}

	// Sonnet 4.5 (legacy) — both ID and alias should be present
	if _, ok := byID["claude-sonnet-4-5-20250929"]; !ok {
		t.Error("claude-sonnet-4-5-20250929 not found")
	}
	if m, ok := byID["claude-sonnet-4-5"]; !ok {
		t.Error("claude-sonnet-4-5 alias not found")
	} else {
		if m.InputPricePer1M != 3 {
			t.Errorf("sonnet 4.5 alias input = %f, want 3", m.InputPricePer1M)
		}
	}
}

func TestParseGrokPage(t *testing.T) {
	// Simulates the RSC payload format found in the xAI docs page
	html := `<html><script>
some stuff \"name\":\"grok-3\",\"version\":\"1.0\",\"inputModalities\":[1],\"outputModalities\":[1],\"promptTextTokenPrice\":\"$n30000\",\"completionTextTokenPrice\":\"$n150000\",\"maxPromptLength\":131072
more stuff \"name\":\"grok-3-mini\",\"version\":\"1.0\",\"promptTextTokenPrice\":\"$n3000\",\"completionTextTokenPrice\":\"$n5000\",\"maxPromptLength\":131072
\"name\":\"grok-4-fast-reasoning\",\"version\":\"1.0\",\"promptTextTokenPrice\":\"$n2000\",\"completionTextTokenPrice\":\"$n5000\",\"maxPromptLength\":2000000
\"name\":\"grok-imagine-image\",\"version\":\"1.0\",\"promptTextTokenPrice\":\"$n0\",\"completionTextTokenPrice\":\"$n0\",\"maxPromptLength\":0
duplicate \"name\":\"grok-3\",\"version\":\"1.0\",\"promptTextTokenPrice\":\"$n30000\",\"completionTextTokenPrice\":\"$n150000\",\"maxPromptLength\":131072
</script></html>`

	models, err := parseGrokPage(html)
	if err != nil {
		t.Fatalf("parseGrokPage error: %v", err)
	}

	byID := make(map[string]ModelPricing)
	for _, m := range models {
		byID[m.ID] = m
	}

	// grok-3: $n30000/10000 = $3.00, $n150000/10000 = $15.00
	if m, ok := byID["grok-3"]; !ok {
		t.Error("grok-3 not found")
	} else {
		if m.InputPricePer1M != 3.0 {
			t.Errorf("grok-3 input = %f, want 3.0", m.InputPricePer1M)
		}
		if m.OutputPricePer1M != 15.0 {
			t.Errorf("grok-3 output = %f, want 15.0", m.OutputPricePer1M)
		}
		if m.ContextWindow != 131072 {
			t.Errorf("grok-3 context = %d, want 131072", m.ContextWindow)
		}
	}

	// grok-4-fast-reasoning: $n2000/10000 = $0.20
	if m, ok := byID["grok-4-fast-reasoning"]; !ok {
		t.Error("grok-4-fast-reasoning not found")
	} else {
		if m.InputPricePer1M != 0.2 {
			t.Errorf("input = %f, want 0.2", m.InputPricePer1M)
		}
		if m.ContextWindow != 2000000 {
			t.Errorf("context = %d, want 2000000", m.ContextWindow)
		}
	}

	// grok-imagine-image should be filtered out
	if _, ok := byID["grok-imagine-image"]; ok {
		t.Error("image model should be filtered out")
	}

	// Duplicates should be deduplicated
	if len(models) != 3 {
		t.Errorf("got %d models, want 3 (deduplicated)", len(models))
	}
}

func TestParseGeminiHTML(t *testing.T) {
	html := `<html><body>
<h3>Gemini 3.1 Pro Preview</h3>
<p>Input price $2.00</p>
<p>Output price $12.00</p>

<h3>Gemini 3 Flash Preview</h3>
<p>Input price $0.50</p>
<p>Output price $3.00</p>

<h3>Gemini 3.1 Flash-Lite Preview</h3>
<p>Input price $0.25</p>
<p>Output price $1.50</p>

<h3>Gemini 3 Flash Image</h3>
<p>Output price $30.00</p>
</body></html>`

	models, err := parseGeminiHTML(html)
	if err != nil {
		t.Fatalf("parseGeminiHTML error: %v", err)
	}

	byID := make(map[string]ModelPricing)
	for _, m := range models {
		byID[m.ID] = m
	}

	if m, ok := byID["gemini-3.1-pro-preview"]; !ok {
		t.Error("gemini-3.1-pro-preview not found")
	} else {
		if m.InputPricePer1M != 2.0 {
			t.Errorf("input = %f, want 2.0", m.InputPricePer1M)
		}
		if m.OutputPricePer1M != 12.0 {
			t.Errorf("output = %f, want 12.0", m.OutputPricePer1M)
		}
	}

	if m, ok := byID["gemini-3-flash-preview"]; !ok {
		t.Error("gemini-3-flash-preview not found")
	} else {
		if m.InputPricePer1M != 0.50 {
			t.Errorf("input = %f, want 0.50", m.InputPricePer1M)
		}
		if m.OutputPricePer1M != 3.0 {
			t.Errorf("output = %f, want 3.0", m.OutputPricePer1M)
		}
	}

	if m, ok := byID["gemini-3.1-flash-lite-preview"]; !ok {
		t.Error("gemini-3.1-flash-lite-preview not found")
	} else {
		if m.InputPricePer1M != 0.25 {
			t.Errorf("input = %f, want 0.25", m.InputPricePer1M)
		}
	}

	// Image model should be skipped
	if _, ok := byID["gemini-3-flash-image"]; ok {
		t.Error("image model should be filtered")
	}
}

func TestParseGeminiSpecPage(t *testing.T) {
	html := `<html><body>
<dt>Model code</dt><dd>gemini-3-flash-preview</dd>
<dt>Input token limit</dt><dd>1,048,576</dd>
<dt>Output token limit</dt><dd>65,536</dd>
</body></html>`

	ctx, maxOut := parseGeminiSpecPage(html)
	if ctx != 1048576 {
		t.Errorf("contextWindow = %d, want 1048576", ctx)
	}
	if maxOut != 65536 {
		t.Errorf("maxOutput = %d, want 65536", maxOut)
	}
}

func TestParseGeminiDeprecations(t *testing.T) {
	html := `<html><body>
<table>
<tr><th>Model</th><th>Release date</th><th>Shutdown date</th><th>Replacement</th></tr>
<tr><td>gemini-3-flash-preview</td><td>December 17, 2025</td><td>No shutdown date announced</td><td>-</td></tr>
<tr><td>gemini-3.1-pro-preview</td><td>February 19, 2026</td><td>No shutdown date announced</td><td>-</td></tr>
<tr><td>gemini-2.5-pro</td><td>June 17, 2025</td><td>June 17, 2026</td><td>gemini-3.1-pro-preview</td></tr>
<tr><td>gemini-2.5-flash</td><td>June 17, 2025</td><td>June 17, 2026</td><td>gemini-3-flash-preview</td></tr>
<tr><td>gemini-2.0-flash</td><td>February 5, 2025</td><td>June 1, 2026</td><td>gemini-3-flash-preview</td></tr>
</table>
</body></html>`

	shutdown := parseGeminiDeprecations(html)

	// Models with a shutdown date should be in the set
	for _, id := range []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"} {
		if !shutdown[id] {
			t.Errorf("expected %s to have shutdown date", id)
		}
	}

	// Models with "No shutdown date announced" should NOT be in the set
	for _, id := range []string{"gemini-3-flash-preview", "gemini-3.1-pro-preview"} {
		if shutdown[id] {
			t.Errorf("expected %s to NOT have shutdown date", id)
		}
	}
}

func TestParseGeminiDeprecations_Empty(t *testing.T) {
	shutdown := parseGeminiDeprecations("<html><body>No tables here</body></html>")
	if len(shutdown) != 0 {
		t.Errorf("expected empty set, got %d entries", len(shutdown))
	}
}

func TestParseGeminiSpecPage_NoData(t *testing.T) {
	ctx, maxOut := parseGeminiSpecPage("<html><body>No specs here</body></html>")
	if ctx != 0 || maxOut != 0 {
		t.Errorf("expected zeros, got ctx=%d, maxOut=%d", ctx, maxOut)
	}
}

func TestParseCommaNumber(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1,048,576", 1048576},
		{"65,536", 65536},
		{"128000", 128000},
		{"0", 0},
	}
	for _, tt := range tests {
		if got := parseCommaNumber(tt.input); got != tt.want {
			t.Errorf("parseCommaNumber(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseHelpers(t *testing.T) {
	t.Run("parseTokenCount", func(t *testing.T) {
		tests := []struct {
			input string
			want  int
		}{
			{"200K tokens", 200000},
			{"1M tokens", 1000000},
			{"128K tokens", 128000},
			{"64K tokens", 64000},
			{"no match", 0},
		}
		for _, tt := range tests {
			if got := parseTokenCount(tt.input); got != tt.want {
				t.Errorf("parseTokenCount(%q) = %d, want %d", tt.input, got, tt.want)
			}
		}
	})

	t.Run("parseSizeStr", func(t *testing.T) {
		tests := []struct {
			input string
			want  int
		}{
			{"2M", 2000000},
			{"256K", 256000},
			{"131K", 131000},
			{"", 0},
		}
		for _, tt := range tests {
			if got := parseSizeStr(tt.input); got != tt.want {
				t.Errorf("parseSizeStr(%q) = %d, want %d", tt.input, got, tt.want)
			}
		}
	})

	t.Run("parseDollarAmount", func(t *testing.T) {
		tests := []struct {
			input string
			want  float64
		}{
			{"$3.00", 3.0},
			{"$0.20", 0.2},
			{"$15.00", 15.0},
			{"no price", 0},
		}
		for _, tt := range tests {
			if got := parseDollarAmount(tt.input); got != tt.want {
				t.Errorf("parseDollarAmount(%q) = %f, want %f", tt.input, got, tt.want)
			}
		}
	})

	t.Run("stripHTMLTags", func(t *testing.T) {
		input := `<strong>Bold</strong> and <a href="x">link</a>`
		got := stripHTMLTags(input)
		if got != "Bold and link" {
			t.Errorf("stripHTMLTags = %q, want %q", got, "Bold and link")
		}
	})
}
