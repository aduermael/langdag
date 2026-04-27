package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"langdag.com/langdag/types"
)

// requireCap skips the current test if the named model does not support the
// requested capability, per the modelCaps table. Feature names: "function_calling",
// "explicit_thinking_budget", "google_search".
func requireCap(t *testing.T, model, feature string) {
	t.Helper()
	c := modelCaps[model]
	var ok bool
	switch feature {
	case "function_calling":
		ok = c.FunctionCalling
	case "explicit_thinking_budget":
		ok = c.ExplicitThinkingBudget
	case "google_search":
		ok = c.GoogleSearch
	default:
		t.Fatalf("requireCap: unknown feature %q", feature)
	}
	if !ok {
		t.Skipf("%s does not support %s", model, feature)
	}
}

// These tests hit the real Google AI Studio API and require GEMINI_API_KEY to be set.
// Run with: GEMINI_API_KEY=<key> go test -v -run TestLive -count=1 ./internal/provider/gemini/
//
// Models tested (all Gemini 3.x without a shutdown date):
//   - gemini-3-flash-preview       (default for most tests, free tier)
//   - gemini-3.1-pro-preview       (pro tier, no free tier)
//   - gemini-3.1-flash-lite-preview (lite, default thinking=minimal)

func liveProvider(t *testing.T) *Provider {
	t.Helper()
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	return New(key)
}

// All active Gemini 3.x models (no shutdown date).
var allModels = []string{
	"gemini-3-flash-preview",
	"gemini-3.1-pro-preview",
	"gemini-3.1-flash-lite-preview",
}

// Gemma models served via the same Google AI Studio endpoint.
var gemmaModels = []string{
	"gemma-4-26b-a4b-it",
	"gemma-3-1b-it",
}

const defaultModel = "gemini-3-flash-preview"

// ---------- Basic completion ----------

// TestLive_SimpleComplete tests a basic synchronous completion across all models.
func TestLive_SimpleComplete(t *testing.T) {
	p := liveProvider(t)

	for _, model := range allModels {
		t.Run(model, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			resp, err := p.Complete(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"Say hello in exactly 3 words."`)},
				},
			})
			if err != nil {
				t.Fatalf("Complete failed: %v", err)
			}

			if len(resp.Content) == 0 {
				t.Fatal("expected at least one content block")
			}
			var hasText bool
			for _, b := range resp.Content {
				if b.Type == "text" && b.Text != "" {
					hasText = true
					t.Logf("response text: %q", b.Text)
				}
			}
			if !hasText {
				t.Error("expected a non-empty text content block")
			}
			t.Logf("usage: in=%d out=%d reasoning=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.ReasoningTokens)
		})
	}
}

// TestLive_SimpleStream tests a basic streaming completion.
func TestLive_SimpleStream(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	events, err := p.Stream(ctx, &types.CompletionRequest{
		Model: defaultModel,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"Say hello in exactly 3 words."`)},
		},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var gotStart, gotDone bool
	var text strings.Builder
	for ev := range events {
		switch ev.Type {
		case types.StreamEventStart:
			gotStart = true
		case types.StreamEventDelta:
			text.WriteString(ev.Content)
		case types.StreamEventDone:
			gotDone = true
			if ev.Response != nil {
				t.Logf("usage: in=%d out=%d reasoning=%d",
					ev.Response.Usage.InputTokens, ev.Response.Usage.OutputTokens, ev.Response.Usage.ReasoningTokens)
			}
		}
	}

	if !gotStart {
		t.Error("missing StreamEventStart")
	}
	if !gotDone {
		t.Error("missing StreamEventDone")
	}
	if text.Len() == 0 {
		t.Error("expected non-empty streamed text")
	}
	t.Logf("streamed text: %q", text.String())
}

// TestLive_LongStreamCoherence forces a response long enough to span many SSE
// chunks and asserts the assembled text contains a distinctive marker phrase
// verbatim. Catches per-chunk delta parsing bugs that drop or chop chunks —
// length-only assertions in TestLive_SimpleStream would not.
func TestLive_LongStreamCoherence(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	const marker = "MAGENTA-LIGHTHOUSE-FORTY-TWO"
	prompt := `Write a 200-word paragraph about lighthouses. Include the exact token "` + marker + `" verbatim somewhere in the middle of the paragraph; do not change its spelling, capitalization, or hyphens.`
	body, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("marshal prompt: %v", err)
	}

	events, err := p.Stream(ctx, &types.CompletionRequest{
		Model:    defaultModel,
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(body)}},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var streamed strings.Builder
	var doneText string
	for ev := range events {
		switch ev.Type {
		case types.StreamEventDelta:
			streamed.WriteString(ev.Content)
		case types.StreamEventDone:
			if ev.Response != nil {
				for _, b := range ev.Response.Content {
					if b.Type == "text" {
						doneText = b.Text
						break
					}
				}
			}
		}
	}

	if !strings.Contains(streamed.String(), marker) {
		t.Errorf("streamed text missing marker %q\ngot (%d chars): %q", marker, streamed.Len(), streamed.String())
	}
	if !strings.Contains(doneText, marker) {
		t.Errorf("done response text missing marker %q\ngot (%d chars): %q", marker, len(doneText), doneText)
	}
	if streamed.String() != doneText {
		t.Errorf("streamed deltas != done response text\nstreamed (%d): %q\ndone (%d): %q",
			streamed.Len(), streamed.String(), len(doneText), doneText)
	}
	t.Logf("streamed %d chars across deltas, marker present", streamed.Len())
}

// TestLive_SystemPrompt tests that the system instruction is respected.
func TestLive_SystemPrompt(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &types.CompletionRequest{
		Model:  defaultModel,
		System: "You are a pirate. Every response must contain the word 'arrr'.",
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"What is 2 + 2?"`)},
		},
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	var text string
	for _, b := range resp.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	t.Logf("response: %q", text)
	if !strings.Contains(strings.ToLower(text), "arrr") {
		t.Log("NOTE: system instruction may not have been followed (no 'arrr' in response)")
	}
}

// ---------- Tool calling ----------

// TestLive_ToolCall tests tool calling across all models.
func TestLive_ToolCall(t *testing.T) {
	p := liveProvider(t)

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}

	for _, model := range allModels {
		t.Run(model, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			resp, err := p.Complete(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"What is the weather in Paris?"`)},
				},
				Tools: tools,
			})
			if err != nil {
				t.Fatalf("Complete failed: %v", err)
			}

			var hasText, hasToolUse bool
			for _, b := range resp.Content {
				switch b.Type {
				case "text":
					hasText = true
					t.Logf("text block: %q", b.Text)
				case "tool_use":
					hasToolUse = true
					t.Logf("tool_use: name=%s args=%s", b.Name, string(b.Input))
				}
			}

			t.Logf("has_text=%v has_tool_use=%v stop_reason=%q", hasText, hasToolUse, resp.StopReason)

			if !hasToolUse {
				if resp.StopReason == "malformed_function_call" {
					t.Logf("NOTE: model generated a malformed function call (server-side issue)")
				} else {
					t.Error("expected a tool_use content block")
				}
			}
			if !hasText {
				t.Logf("NOTE: %s returned tool_use WITHOUT any text block", model)
			}
		})
	}
}

// TestLive_ToolCallStream tests streaming with tool calls.
func TestLive_ToolCallStream(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}

	events, err := p.Stream(ctx, &types.CompletionRequest{
		Model: defaultModel,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"What is the weather in Paris?"`)},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var text strings.Builder
	var toolCalls []types.ContentBlock
	for ev := range events {
		switch ev.Type {
		case types.StreamEventDelta:
			text.WriteString(ev.Content)
		case types.StreamEventContentDone:
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				toolCalls = append(toolCalls, *ev.ContentBlock)
			}
		case types.StreamEventDone:
			if ev.Response != nil {
				t.Logf("usage: in=%d out=%d", ev.Response.Usage.InputTokens, ev.Response.Usage.OutputTokens)
			}
		}
	}

	t.Logf("streamed_text=%q tool_calls=%d", text.String(), len(toolCalls))
	for _, tc := range toolCalls {
		t.Logf("  tool: name=%s args=%s", tc.Name, string(tc.Input))
	}

	if len(toolCalls) == 0 {
		t.Error("expected at least one tool call")
	}
}

// TestLive_MultiTurnToolUse tests a full tool-use round trip:
// user asks → model calls tool → we provide result → model responds with text.
func TestLive_MultiTurnToolUse(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}

	// Turn 1: user asks, model should call tool
	resp1, err := p.Complete(ctx, &types.CompletionRequest{
		Model: defaultModel,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"What is the weather in Tokyo?"`)},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("Turn 1 failed: %v", err)
	}

	var toolCall *types.ContentBlock
	for i := range resp1.Content {
		if resp1.Content[i].Type == "tool_use" {
			toolCall = &resp1.Content[i]
			break
		}
	}
	if toolCall == nil {
		t.Fatal("Turn 1: expected a tool_use block")
	}
	t.Logf("Turn 1: tool=%s args=%s", toolCall.Name, string(toolCall.Input))

	// Build assistant message with all content blocks from turn 1
	assistantContent, _ := json.Marshal(resp1.Content)

	// Build tool result — use toolCall.ID (unique call ID for Gemini 3.x,
	// falls back to function name for older models).
	toolResult := []types.ContentBlock{
		{
			Type:      "tool_result",
			ToolUseID: toolCall.ID,
			Content:   `{"temperature": "22°C", "condition": "sunny", "humidity": "45%"}`,
		},
	}
	toolResultJSON, _ := json.Marshal(toolResult)

	// Turn 2: provide tool result, model should respond with text
	resp2, err := p.Complete(ctx, &types.CompletionRequest{
		Model: defaultModel,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"What is the weather in Tokyo?"`)},
			{Role: "assistant", Content: json.RawMessage(assistantContent)},
			{Role: "user", Content: json.RawMessage(toolResultJSON)},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("Turn 2 failed: %v", err)
	}

	var hasText bool
	for _, b := range resp2.Content {
		if b.Type == "text" && b.Text != "" {
			hasText = true
			t.Logf("Turn 2 text: %q", b.Text)
		}
	}
	if !hasText {
		t.Error("Turn 2: expected text response after tool result")
	}
	t.Logf("Turn 2 usage: in=%d out=%d", resp2.Usage.InputTokens, resp2.Usage.OutputTokens)
}

// TestLive_ConsecutiveToolCalls tests if the model batches multiple tool calls.
func TestLive_ConsecutiveToolCalls(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
		{
			Name:        "get_time",
			Description: "Get the current time in a timezone",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"timezone":{"type":"string"}},"required":["timezone"]}`),
		},
	}

	resp, err := p.Complete(ctx, &types.CompletionRequest{
		Model: defaultModel,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"What is the weather and current time in London?"`)},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	var toolCalls int
	for _, b := range resp.Content {
		if b.Type == "tool_use" {
			toolCalls++
			t.Logf("tool_use: name=%s args=%s", b.Name, string(b.Input))
		}
		if b.Type == "text" {
			t.Logf("text: %q", truncate(b.Text, 200))
		}
	}

	t.Logf("tool_calls=%d stop=%q", toolCalls, resp.StopReason)
	if toolCalls > 1 {
		t.Log("NOTE: model batched multiple tool calls in one response")
	} else if toolCalls == 1 {
		t.Log("NOTE: model returned only one tool call (no batching)")
	}
}

// ---------- Thinking / reasoning ----------

// TestLive_Thinking tests thinking configuration across all models.
// Gemini 3.x uses thinkingLevel (minimal/low/medium/high) but still accepts
// thinkingBudget for backward compat. Our protocol sends thinkingBudget — this
// test verifies whether that still works.
func TestLive_Thinking(t *testing.T) {
	p := liveProvider(t)

	for _, model := range allModels {
		t.Run(model, func(t *testing.T) {
			t.Run("explicit_thinking_enabled", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer cancel()

				think := true
				resp, err := p.Complete(ctx, &types.CompletionRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: json.RawMessage(`"What is 17 * 23?"`)},
					},
					Think: &think,
				})
				if err != nil {
					if strings.Contains(err.Error(), "400") {
						t.Logf("explicit thinking rejected (thinkingBudget may not be accepted): %v", err)
					} else if strings.Contains(err.Error(), "503") {
						t.Skipf("server unavailable (transient): %v", err)
					} else {
						t.Fatalf("unexpected error: %v", err)
					}
					return
				}

				t.Logf("usage: in=%d out=%d reasoning=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.ReasoningTokens)
				for _, b := range resp.Content {
					if b.Type == "text" {
						t.Logf("response: %q", b.Text)
					}
				}
			})

			t.Run("explicit_thinking_disabled", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer cancel()

				think := false
				resp, err := p.Complete(ctx, &types.CompletionRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: json.RawMessage(`"What is 17 * 23?"`)},
					},
					Think: &think,
				})
				if err != nil {
					if strings.Contains(err.Error(), "400") {
						t.Logf("disabling thinking rejected: %v", err)
					} else if strings.Contains(err.Error(), "503") {
						t.Skipf("server unavailable (transient): %v", err)
					} else {
						t.Fatalf("unexpected error: %v", err)
					}
					return
				}

				t.Logf("usage: in=%d out=%d reasoning=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.ReasoningTokens)
				if resp.Usage.ReasoningTokens > 0 {
					t.Log("NOTE: reasoning tokens reported even with thinking disabled")
				}
				for _, b := range resp.Content {
					if b.Type == "text" {
						t.Logf("response: %q", b.Text)
					}
				}
			})

			t.Run("default_no_thinking_config", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer cancel()

				resp, err := p.Complete(ctx, &types.CompletionRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: json.RawMessage(`"What is 17 * 23?"`)},
					},
				})
				if err != nil {
					t.Fatalf("Complete failed: %v", err)
				}

				t.Logf("usage: in=%d out=%d reasoning=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.ReasoningTokens)
				if resp.Usage.ReasoningTokens > 0 {
					t.Logf("NOTE: %s reports reasoning tokens by default", model)
				}
				for _, b := range resp.Content {
					if b.Type == "text" {
						t.Logf("response: %q", b.Text)
					}
				}
			})
		})
	}
}

// TestLive_ThinkingStream tests that streaming correctly filters thought parts
// and still delivers text deltas with a thinking model.
func TestLive_ThinkingStream(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	think := true
	events, err := p.Stream(ctx, &types.CompletionRequest{
		Model: defaultModel,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"Explain why the sky is blue in one sentence."`)},
		},
		Think: &think,
	})
	if err != nil {
		if strings.Contains(err.Error(), "400") {
			t.Skipf("thinking not supported via thinkingBudget: %v", err)
		}
		t.Fatalf("Stream failed: %v", err)
	}

	var gotStart, gotDone bool
	var text strings.Builder
	for ev := range events {
		switch ev.Type {
		case types.StreamEventStart:
			gotStart = true
		case types.StreamEventDelta:
			text.WriteString(ev.Content)
		case types.StreamEventDone:
			gotDone = true
			if ev.Response != nil {
				t.Logf("usage: in=%d out=%d reasoning=%d",
					ev.Response.Usage.InputTokens, ev.Response.Usage.OutputTokens, ev.Response.Usage.ReasoningTokens)
			}
		}
	}

	if !gotStart {
		t.Error("missing StreamEventStart")
	}
	if !gotDone {
		t.Error("missing StreamEventDone")
	}
	if text.Len() == 0 {
		t.Error("expected non-empty streamed text (thought parts should be filtered)")
	}
	t.Logf("streamed text: %q", text.String())
}

// ---------- Large context ----------

// TestLive_LargeContext tests behavior with a large input context.
func TestLive_LargeContext(t *testing.T) {
	p := liveProvider(t)

	filler := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 4000)

	sizes := []struct {
		name  string
		chars int
	}{
		{"10k_chars", 10_000},
		{"50k_chars", 50_000},
		{"200k_chars", 200_000},
	}

	for _, sz := range sizes {
		t.Run(sz.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			content := filler
			if len(content) > sz.chars {
				content = content[:sz.chars]
			}

			msg := "Here is some context:\n" + content + "\n\nSummarize the above in one sentence."
			msgJSON, _ := json.Marshal(msg)

			resp, err := p.Complete(ctx, &types.CompletionRequest{
				Model: defaultModel,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(msgJSON)},
				},
			})
			if err != nil {
				t.Logf("ERROR with %s context: %v", sz.name, err)
				if strings.Contains(err.Error(), "500") {
					t.Logf("GOT 500 ERROR — context size may be the cause")
				}
				return
			}

			t.Logf("OK: in=%d out=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
			for _, b := range resp.Content {
				if b.Type == "text" {
					t.Logf("response: %q", truncate(b.Text, 200))
				}
			}
		})
	}
}

// ---------- Gemma-specific tests ----------

// TestLive_Gemma_SimpleComplete tests a basic completion with a Gemma model.
func TestLive_Gemma_SimpleComplete(t *testing.T) {
	p := liveProvider(t)

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			resp, err := p.Complete(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"Say hello in exactly 3 words."`)},
				},
			})
			if err != nil {
				t.Fatalf("Complete failed: %v", err)
			}

			if len(resp.Content) == 0 {
				t.Fatal("expected at least one content block")
			}
			var hasText bool
			for _, b := range resp.Content {
				if b.Type == "text" && b.Text != "" {
					hasText = true
					t.Logf("response text: %q", b.Text)
				}
			}
			if !hasText {
				t.Error("expected a non-empty text content block")
			}
			t.Logf("usage: in=%d out=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
		})
	}
}

// TestLive_Gemma_SimpleStream tests a basic streaming completion with a Gemma model.
func TestLive_Gemma_SimpleStream(t *testing.T) {
	p := liveProvider(t)

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			events, err := p.Stream(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"Say hello in exactly 3 words."`)},
				},
			})
			if err != nil {
				t.Fatalf("Stream failed: %v", err)
			}

			var gotStart, gotDone bool
			var text strings.Builder
			for ev := range events {
				switch ev.Type {
				case types.StreamEventStart:
					gotStart = true
				case types.StreamEventDelta:
					text.WriteString(ev.Content)
				case types.StreamEventDone:
					gotDone = true
					if ev.Response != nil {
						t.Logf("usage: in=%d out=%d", ev.Response.Usage.InputTokens, ev.Response.Usage.OutputTokens)
					}
				}
			}

			if !gotStart {
				t.Error("missing StreamEventStart")
			}
			if !gotDone {
				t.Error("missing StreamEventDone")
			}
			if text.Len() == 0 {
				t.Error("expected non-empty streamed text")
			}
			t.Logf("streamed text: %q", text.String())
		})
	}
}

// TestLive_Gemma_ToolCall tests tool calling with a Gemma model.
func TestLive_Gemma_ToolCall(t *testing.T) {
	p := liveProvider(t)

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			requireCap(t, model, "function_calling")
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			resp, err := p.Complete(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"What is the weather in Paris?"`)},
				},
				Tools: tools,
			})
			if err != nil {
				t.Fatalf("Complete failed: %v", err)
			}

			var hasText, hasToolUse bool
			for _, b := range resp.Content {
				switch b.Type {
				case "text":
					hasText = true
					t.Logf("text block: %q", b.Text)
				case "tool_use":
					hasToolUse = true
					t.Logf("tool_use: name=%s args=%s", b.Name, string(b.Input))
				}
			}

			t.Logf("has_text=%v has_tool_use=%v stop_reason=%q", hasText, hasToolUse, resp.StopReason)
			if !hasToolUse {
				t.Error("expected a tool_use content block")
			}
			if !hasText {
				t.Log("NOTE: Gemma returned tool_use WITHOUT any text block")
			}
		})
	}
}

// TestLive_Gemma_ToolCallStream tests streaming with tool calls for a Gemma model.
func TestLive_Gemma_ToolCallStream(t *testing.T) {
	p := liveProvider(t)

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			requireCap(t, model, "function_calling")
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			events, err := p.Stream(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"What is the weather in Paris?"`)},
				},
				Tools: tools,
			})
			if err != nil {
				t.Fatalf("Stream failed: %v", err)
			}

			var text strings.Builder
			var toolCalls []types.ContentBlock
			for ev := range events {
				switch ev.Type {
				case types.StreamEventDelta:
					text.WriteString(ev.Content)
				case types.StreamEventContentDone:
					if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
						toolCalls = append(toolCalls, *ev.ContentBlock)
					}
				case types.StreamEventDone:
					if ev.Response != nil {
						t.Logf("usage: in=%d out=%d", ev.Response.Usage.InputTokens, ev.Response.Usage.OutputTokens)
					}
				}
			}

			t.Logf("streamed_text=%q tool_calls=%d", text.String(), len(toolCalls))
			for _, tc := range toolCalls {
				t.Logf("  tool: name=%s args=%s", tc.Name, string(tc.Input))
			}

			if len(toolCalls) == 0 {
				t.Error("expected at least one tool call")
			}
			if text.Len() == 0 {
				t.Log("NOTE: Gemma streamed tool calls WITHOUT any text deltas")
			}
		})
	}
}

// TestLive_Gemma_MultiTurnToolUse tests a full tool-use round trip with a Gemma model:
// user asks → model calls tool → we provide result → model responds with text.
func TestLive_Gemma_MultiTurnToolUse(t *testing.T) {
	p := liveProvider(t)

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			requireCap(t, model, "function_calling")
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			// Turn 1: user asks, model should call tool
			resp1, err := p.Complete(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"What is the weather in Tokyo?"`)},
				},
				Tools: tools,
			})
			if err != nil {
				t.Fatalf("Turn 1 failed: %v", err)
			}

			var toolCall *types.ContentBlock
			for i := range resp1.Content {
				if resp1.Content[i].Type == "tool_use" {
					toolCall = &resp1.Content[i]
					break
				}
			}
			if toolCall == nil {
				t.Fatal("Turn 1: expected a tool_use block")
			}
			t.Logf("Turn 1: tool=%s args=%s", toolCall.Name, string(toolCall.Input))

			// Build assistant message with all content blocks from turn 1
			assistantContent, _ := json.Marshal(resp1.Content)

			// Build tool result — use toolCall.ID (unique call ID for Gemini 3.x,
			// falls back to function name for older models).
			toolResult := []types.ContentBlock{
				{
					Type:      "tool_result",
					ToolUseID: toolCall.ID,
					Content:   `{"temperature": "22°C", "condition": "sunny", "humidity": "45%"}`,
				},
			}
			toolResultJSON, _ := json.Marshal(toolResult)

			// Turn 2: provide tool result, model should respond with text
			resp2, err := p.Complete(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"What is the weather in Tokyo?"`)},
					{Role: "assistant", Content: json.RawMessage(assistantContent)},
					{Role: "user", Content: json.RawMessage(toolResultJSON)},
				},
				Tools: tools,
			})
			if err != nil {
				t.Fatalf("Turn 2 failed: %v", err)
			}

			var hasText bool
			for _, b := range resp2.Content {
				if b.Type == "text" && b.Text != "" {
					hasText = true
					t.Logf("Turn 2 text: %q", b.Text)
				}
			}
			if !hasText {
				t.Error("Turn 2: expected text response after tool result")
			}
			t.Logf("Turn 2 usage: in=%d out=%d", resp2.Usage.InputTokens, resp2.Usage.OutputTokens)
		})
	}
}

// TestLive_Gemma_ConsecutiveToolCalls tests if a Gemma model batches multiple
// tool calls in a single response.
func TestLive_Gemma_ConsecutiveToolCalls(t *testing.T) {
	p := liveProvider(t)

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
		{
			Name:        "get_time",
			Description: "Get the current time in a timezone",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"timezone":{"type":"string"}},"required":["timezone"]}`),
		},
	}

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			requireCap(t, model, "function_calling")
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			resp, err := p.Complete(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"What is the weather and current time in London?"`)},
				},
				Tools: tools,
			})
			if err != nil {
				t.Fatalf("Complete failed: %v", err)
			}

			var toolCalls int
			for _, b := range resp.Content {
				if b.Type == "tool_use" {
					toolCalls++
					t.Logf("tool_use: name=%s args=%s", b.Name, string(b.Input))
				}
				if b.Type == "text" {
					t.Logf("text: %q", truncate(b.Text, 200))
				}
			}

			t.Logf("tool_calls=%d stop=%q", toolCalls, resp.StopReason)
			if toolCalls > 1 {
				t.Log("NOTE: Gemma batched multiple tool calls in one response")
			} else if toolCalls == 1 {
				t.Log("NOTE: Gemma returned only one tool call (no batching)")
			}
		})
	}
}

// TestLive_Gemma_LargeContext tests behavior with a large input context for a Gemma model.
func TestLive_Gemma_LargeContext(t *testing.T) {
	p := liveProvider(t)

	filler := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 4000)

	sizes := []struct {
		name  string
		chars int
	}{
		{"10k_chars", 10_000},
		{"50k_chars", 50_000},
		{"200k_chars", 200_000},
	}

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			for _, sz := range sizes {
				t.Run(sz.name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
					defer cancel()

					content := filler
					if len(content) > sz.chars {
						content = content[:sz.chars]
					}

					msg := "Here is some context:\n" + content + "\n\nSummarize the above in one sentence."
					msgJSON, _ := json.Marshal(msg)

					resp, err := p.Complete(ctx, &types.CompletionRequest{
						Model: model,
						Messages: []types.Message{
							{Role: "user", Content: json.RawMessage(msgJSON)},
						},
					})
					if err != nil {
						t.Logf("ERROR with %s context: %v", sz.name, err)
						if strings.Contains(err.Error(), "500") {
							t.Logf("GOT 500 ERROR — context size may be the cause")
						}
						return
					}

					t.Logf("OK: in=%d out=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
					for _, b := range resp.Content {
						if b.Type == "text" {
							t.Logf("response: %q", truncate(b.Text, 200))
						}
					}
				})
			}
		})
	}
}

// TestLive_Gemma_ToolCallWithReasoningTokens tests tool calling and checks
// whether Gemma reports reasoning tokens alongside tool calls.
func TestLive_Gemma_ToolCallWithReasoningTokens(t *testing.T) {
	p := liveProvider(t)

	tools := []types.ToolDefinition{
		{
			Name:        "search_code",
			Description: "Search for a pattern in source code files",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`),
		},
	}

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			requireCap(t, model, "function_calling")
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			resp, err := p.Complete(ctx, &types.CompletionRequest{
				Model: model,
				Messages: []types.Message{
					{Role: "user", Content: json.RawMessage(`"Find where the git branch name is displayed in the UI"`)},
				},
				Tools: tools,
			})
			if err != nil {
				t.Fatalf("Complete failed: %v", err)
			}

			var hasText, hasToolUse bool
			for _, b := range resp.Content {
				switch b.Type {
				case "text":
					hasText = true
					t.Logf("text: %q", truncate(b.Text, 200))
				case "tool_use":
					hasToolUse = true
					t.Logf("tool_use: name=%s args=%s", b.Name, string(b.Input))
				}
			}

			t.Logf("has_text=%v has_tool_use=%v stop=%q reasoning=%d",
				hasText, hasToolUse, resp.StopReason, resp.Usage.ReasoningTokens)
			if resp.Usage.ReasoningTokens > 0 {
				t.Log("NOTE: reasoning tokens reported alongside tool calls without explicit thinking")
			}
		})
	}
}

// TestLive_Gemma_Thinking tests that Gemma handles thinking config correctly.
func TestLive_Gemma_Thinking(t *testing.T) {
	p := liveProvider(t)

	for _, model := range gemmaModels {
		t.Run(model, func(t *testing.T) {
			t.Run("explicit_thinking_refused_client_side", func(t *testing.T) {
				if modelCaps[model].ExplicitThinkingBudget {
					t.Skipf("%s supports explicit thinking budget; nothing to refuse", model)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				think := true
				_, err := p.Complete(ctx, &types.CompletionRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: json.RawMessage(`"What is 17 * 23?"`)},
					},
					Think: &think,
				})
				var fe *ErrFeatureUnsupported
				if !errors.As(err, &fe) {
					t.Fatalf("expected *ErrFeatureUnsupported, got: %v", err)
				}
				if fe.Feature != "explicit_thinking_budget" {
					t.Errorf("expected Feature=%q, got %q", "explicit_thinking_budget", fe.Feature)
				}
				if fe.Model != model {
					t.Errorf("expected Model=%q, got %q", model, fe.Model)
				}
				t.Logf("confirmed client-side refusal: %v", err)
			})

			t.Run("implicit_reasoning_tokens", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer cancel()

				resp, err := p.Complete(ctx, &types.CompletionRequest{
					Model: model,
					Messages: []types.Message{
						{Role: "user", Content: json.RawMessage(`"What is 17 * 23?"`)},
					},
				})
				if err != nil {
					t.Fatalf("Complete failed: %v", err)
				}

				t.Logf("usage: in=%d out=%d reasoning=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.ReasoningTokens)
				if resp.Usage.ReasoningTokens > 0 {
					t.Log("NOTE: model reports reasoning tokens even without explicit thinking config")
				}
				for _, b := range resp.Content {
					if b.Type == "text" {
						t.Logf("response: %q", b.Text)
					}
				}
			})
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
