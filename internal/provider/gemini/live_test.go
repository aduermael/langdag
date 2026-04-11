package gemini

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"langdag.com/langdag/types"
)

// These tests hit the real Gemini API and require GEMINI_API_KEY to be set.
// Run with: go test -v -run TestLive ./internal/provider/gemini/

func liveProvider(t *testing.T) *Provider {
	t.Helper()
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	return New(key)
}

const defaultModel = "gemini-2.0-flash"

// TestLive_SimpleComplete tests a basic synchronous completion.
func TestLive_SimpleComplete(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &types.CompletionRequest{
		Model: defaultModel,
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
}

// TestLive_SimpleStream tests a basic streaming completion.
func TestLive_SimpleStream(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
}

// TestLive_ToolCall tests whether Gemini returns text alongside tool calls.
func TestLive_ToolCall(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a city",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		},
	}

	resp, err := p.Complete(ctx, &types.CompletionRequest{
		Model: defaultModel,
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
	t.Logf("usage: in=%d out=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)

	if !hasToolUse {
		t.Error("expected a tool_use content block")
	}
	if !hasText {
		t.Log("NOTE: Gemini returned tool_use WITHOUT any text block")
	}
}

// TestLive_ToolCallStream tests streaming with tool calls.
func TestLive_ToolCallStream(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	if text.Len() == 0 {
		t.Log("NOTE: Gemini streamed tool calls WITHOUT any text deltas")
	}
}

// TestLive_MultiTurnToolUse tests a full tool-use round trip:
// user asks → model calls tool → we provide result → model responds with text.
func TestLive_MultiTurnToolUse(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	// Build tool result
	toolResult := []types.ContentBlock{
		{
			Type:      "tool_result",
			ToolUseID: toolCall.Name,
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

// TestLive_Thinking tests completion with thinking/reasoning enabled.
// Unlike Gemma, Gemini 2.5 Pro supports explicit thinkingBudget configuration.
func TestLive_Thinking(t *testing.T) {
	p := liveProvider(t)

	t.Run("explicit_thinking", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		think := true
		resp, err := p.Complete(ctx, &types.CompletionRequest{
			Model: "gemini-2.5-pro-preview-05-06",
			Messages: []types.Message{
				{Role: "user", Content: json.RawMessage(`"What is 17 * 23?"`)},
			},
			Think: &think,
		})
		if err != nil {
			if strings.Contains(err.Error(), "400") {
				t.Logf("explicit thinking rejected: %v", err)
			} else {
				t.Fatalf("unexpected error: %v", err)
			}
			return
		}

		t.Logf("usage: in=%d out=%d reasoning=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.ReasoningTokens)
		if resp.Usage.ReasoningTokens > 0 {
			t.Log("explicit thinking produced reasoning tokens")
		}
		for _, b := range resp.Content {
			if b.Type == "text" {
				t.Logf("response: %q", b.Text)
			}
		}
	})

	t.Run("implicit_reasoning_tokens", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := p.Complete(ctx, &types.CompletionRequest{
			Model: defaultModel,
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
}

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
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

// TestLive_ToolCallWithReasoningTokens tests tool calling and checks whether
// the model reports reasoning tokens alongside tool calls.
func TestLive_ToolCallWithReasoningTokens(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tools := []types.ToolDefinition{
		{
			Name:        "search_code",
			Description: "Search for a pattern in source code files",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`),
		},
	}

	resp, err := p.Complete(ctx, &types.CompletionRequest{
		Model: defaultModel,
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
		t.Log("NOTE: reasoning tokens reported alongside tool calls")
	}
}

// TestLive_ConsecutiveToolCalls tests if the model batches multiple tool calls
// in a single response when multiple tools are available.
func TestLive_ConsecutiveToolCalls(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		t.Log("NOTE: Gemini batched multiple tool calls in one response")
	} else if toolCalls == 1 {
		t.Log("NOTE: Gemini returned only one tool call (no batching)")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
