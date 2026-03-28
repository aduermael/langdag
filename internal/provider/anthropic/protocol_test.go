package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"langdag.com/langdag/types"
)

// mockDecoder implements ssestream.Decoder for testing processStreamEvents.
type mockDecoder struct {
	events []ssestream.Event
	idx    int
}

func (d *mockDecoder) Next() bool {
	if d.idx >= len(d.events) {
		return false
	}
	d.idx++
	return true
}

func (d *mockDecoder) Event() ssestream.Event {
	return d.events[d.idx-1]
}

func (d *mockDecoder) Err() error   { return nil }
func (d *mockDecoder) Close() error { return nil }

// makeEvent creates an SSE event with the given type and JSON data.
func makeEvent(eventType string, data interface{}) ssestream.Event {
	b, _ := json.Marshal(data)
	return ssestream.Event{Type: eventType, Data: b}
}

// ---------------------------------------------------------------------------
// buildParams — prompt caching (CacheControl on system prompt and last tool)
// ---------------------------------------------------------------------------

func TestBuildParams_SystemPromptHasCacheControl(t *testing.T) {
	req := &types.CompletionRequest{
		Model:     "claude-sonnet-4-20250514",
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		System:    "You are a helpful assistant.",
		MaxTokens: 1024,
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if len(params.System) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(params.System))
	}
	if params.System[0].CacheControl.Type != "ephemeral" {
		t.Errorf("system CacheControl.Type = %q, want %q", params.System[0].CacheControl.Type, "ephemeral")
	}
}

func TestBuildParams_NoSystemPromptNoCacheControl(t *testing.T) {
	req := &types.CompletionRequest{
		Model:     "claude-sonnet-4-20250514",
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		MaxTokens: 1024,
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if len(params.System) != 0 {
		t.Errorf("expected no system blocks when system prompt is empty, got %d", len(params.System))
	}
}

func TestBuildParams_LastClientToolHasCacheControl(t *testing.T) {
	req := &types.CompletionRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		Tools: []types.ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
			},
			{
				Name:        "calculator",
				Description: "Calculate",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"expr":{"type":"string"}}}`),
			},
		},
		MaxTokens: 1024,
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if len(params.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(params.Tools))
	}
	// First tool should NOT have CacheControl set
	if params.Tools[0].OfTool.CacheControl.Type == "ephemeral" {
		t.Error("first tool should not have CacheControl set")
	}
	// Last tool SHOULD have CacheControl set
	if params.Tools[1].OfTool.CacheControl.Type != "ephemeral" {
		t.Errorf("last tool CacheControl.Type = %q, want %q", params.Tools[1].OfTool.CacheControl.Type, "ephemeral")
	}
}

func TestBuildParams_LastServerToolHasCacheControl(t *testing.T) {
	req := &types.CompletionRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		Tools: []types.ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
			},
			{Name: types.ServerToolWebSearch},
		},
		MaxTokens: 1024,
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if len(params.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(params.Tools))
	}
	// First tool (client) should NOT have CacheControl
	if params.Tools[0].OfTool.CacheControl.Type == "ephemeral" {
		t.Error("first tool should not have CacheControl set")
	}
	// Last tool (server/web_search) SHOULD have CacheControl
	if params.Tools[1].OfWebSearchTool20250305 == nil {
		t.Fatal("expected last tool to be web_search server tool")
	}
	if params.Tools[1].OfWebSearchTool20250305.CacheControl.Type != "ephemeral" {
		t.Errorf("last server tool CacheControl.Type = %q, want %q",
			params.Tools[1].OfWebSearchTool20250305.CacheControl.Type, "ephemeral")
	}
}

func TestBuildParams_SingleToolHasCacheControl(t *testing.T) {
	req := &types.CompletionRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		Tools: []types.ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
			},
		},
		MaxTokens: 1024,
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if len(params.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(params.Tools))
	}
	if params.Tools[0].OfTool.CacheControl.Type != "ephemeral" {
		t.Errorf("single tool CacheControl.Type = %q, want %q", params.Tools[0].OfTool.CacheControl.Type, "ephemeral")
	}
}

func TestBuildParams_SystemAndToolsBothCached(t *testing.T) {
	req := &types.CompletionRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		System:   "You are a helpful assistant.",
		Tools: []types.ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
			},
		},
		MaxTokens: 1024,
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	// System prompt should be cached
	if params.System[0].CacheControl.Type != "ephemeral" {
		t.Error("system prompt should have CacheControl set")
	}
	// Tool should also be cached
	if params.Tools[0].OfTool.CacheControl.Type != "ephemeral" {
		t.Error("tool should have CacheControl set")
	}
}

// ---------------------------------------------------------------------------
// buildParams — extended thinking
// ---------------------------------------------------------------------------

func boolPtr(v bool) *bool { return &v }

func TestBuildParams_ThinkEnabled(t *testing.T) {
	req := &types.CompletionRequest{
		Model:       "claude-sonnet-4-20250514",
		Messages:    []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		MaxTokens:   1024,
		Temperature: 0.7,
		Think:       boolPtr(true),
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	// Thinking should be enabled with the default budget.
	if params.Thinking.OfEnabled == nil {
		t.Fatal("expected Thinking.OfEnabled to be set")
	}
	if params.Thinking.OfEnabled.BudgetTokens != 10240 {
		t.Errorf("BudgetTokens = %d, want 10240", params.Thinking.OfEnabled.BudgetTokens)
	}

	// MaxTokens must be bumped to at least budget + 4096.
	if params.MaxTokens < 10240+4096 {
		t.Errorf("MaxTokens = %d, want >= %d", params.MaxTokens, 10240+4096)
	}

	// Temperature must NOT be set when thinking is enabled.
	if params.Temperature.Value != 0 {
		t.Errorf("Temperature should be unset when thinking is enabled, got %v", params.Temperature.Value)
	}
}

func TestBuildParams_ThinkEnabled_LargeMaxTokens(t *testing.T) {
	// When the caller already provides a MaxTokens larger than budget + 4096,
	// it should be left unchanged.
	req := &types.CompletionRequest{
		Model:     "claude-sonnet-4-20250514",
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		MaxTokens: 32768,
		Think:     boolPtr(true),
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if params.MaxTokens != 32768 {
		t.Errorf("MaxTokens = %d, want 32768 (should not be reduced)", params.MaxTokens)
	}
}

func TestBuildParams_ThinkFalse(t *testing.T) {
	req := &types.CompletionRequest{
		Model:       "claude-sonnet-4-20250514",
		Messages:    []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		MaxTokens:   1024,
		Temperature: 0.5,
		Think:       boolPtr(false),
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	// Thinking should not be configured.
	if params.Thinking.OfEnabled != nil {
		t.Error("expected Thinking.OfEnabled to be nil when Think=false")
	}

	// Temperature should still be set.
	if params.Temperature.Value != 0.5 {
		t.Errorf("Temperature = %v, want 0.5", params.Temperature.Value)
	}

	// MaxTokens should remain as-is.
	if params.MaxTokens != 1024 {
		t.Errorf("MaxTokens = %d, want 1024", params.MaxTokens)
	}
}

func TestBuildParams_ThinkNil(t *testing.T) {
	req := &types.CompletionRequest{
		Model:       "claude-sonnet-4-20250514",
		Messages:    []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		MaxTokens:   2048,
		Temperature: 0.9,
	}
	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	// Thinking should not be configured.
	if params.Thinking.OfEnabled != nil {
		t.Error("expected Thinking.OfEnabled to be nil when Think is nil")
	}

	// Temperature should be set normally.
	if params.Temperature.Value != 0.9 {
		t.Errorf("Temperature = %v, want 0.9", params.Temperature.Value)
	}
}

func TestConvertTools_FunctionOnly(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
		},
	}
	result, err := convertTools(tools)
	if err != nil {
		t.Fatalf("convertTools: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].OfTool == nil {
		t.Fatal("expected OfTool to be set for function tool")
	}
	if result[0].OfWebSearchTool20250305 != nil {
		t.Fatal("expected OfWebSearchTool20250305 to be nil for function tool")
	}
	if result[0].OfTool.Name != "get_weather" {
		t.Errorf("tool name = %q, want %q", result[0].OfTool.Name, "get_weather")
	}
}

func TestConvertTools_ServerToolWebSearch(t *testing.T) {
	tools := []types.ToolDefinition{
		{Name: types.ServerToolWebSearch},
	}
	result, err := convertTools(tools)
	if err != nil {
		t.Fatalf("convertTools: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].OfWebSearchTool20250305 == nil {
		t.Fatal("expected OfWebSearchTool20250305 to be set for web_search")
	}
	if result[0].OfTool != nil {
		t.Fatal("expected OfTool to be nil for server tool")
	}
}

func TestConvertTools_MixedFunctionAndServerTools(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
		},
		{Name: types.ServerToolWebSearch},
		{
			Name:        "calculator",
			Description: "Calculate",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"expr":{"type":"string"}}}`),
		},
	}
	result, err := convertTools(tools)
	if err != nil {
		t.Fatalf("convertTools: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(result))
	}

	// First: function tool
	if result[0].OfTool == nil || result[0].OfTool.Name != "get_weather" {
		t.Error("expected first tool to be get_weather function")
	}

	// Second: server tool
	if result[1].OfWebSearchTool20250305 == nil {
		t.Error("expected second tool to be web_search server tool")
	}

	// Third: function tool
	if result[2].OfTool == nil || result[2].OfTool.Name != "calculator" {
		t.Error("expected third tool to be calculator function")
	}
}

func TestConvertTools_UnknownServerToolSkipped(t *testing.T) {
	// A tool without InputSchema and an unknown name should be silently skipped
	tools := []types.ToolDefinition{
		{
			Name:        "custom_search",
			Description: "Custom search",
		},
	}
	result, err := convertTools(tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 tools after skipping unknown server tool, got %d", len(result))
	}
}

func TestConvertTools_WebSearchWithSchemaIsClientTool(t *testing.T) {
	// A tool named "web_search" but with InputSchema should be treated as a client (function) tool
	tools := []types.ToolDefinition{
		{
			Name:        "web_search",
			Description: "Custom web search override",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		},
	}
	result, err := convertTools(tools)
	if err != nil {
		t.Fatalf("convertTools: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].OfTool == nil {
		t.Fatal("expected OfTool to be set for client tool with schema")
	}
	if result[0].OfWebSearchTool20250305 != nil {
		t.Fatal("expected OfWebSearchTool20250305 to be nil for client tool with schema")
	}
	if result[0].OfTool.Name != "web_search" {
		t.Errorf("tool name = %q, want %q", result[0].OfTool.Name, "web_search")
	}
}

func TestConvertMessages_ToolUseBlocks(t *testing.T) {
	// Simulate an assistant message with tool_use blocks (e.g. stored from a non-Anthropic provider)
	blocks := []types.ContentBlock{
		{Type: "text", Text: "Let me check the weather."},
		{
			Type:  "tool_use",
			ID:    "toolu_abc123",
			Name:  "get_weather",
			Input: json.RawMessage(`{"location":"Paris"}`),
		},
	}
	blocksJSON, _ := json.Marshal(blocks)

	messages := []types.Message{
		{Role: "assistant", Content: blocksJSON},
	}

	result, err := convertMessages(messages)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(result[0].Content))
	}

	// First block: text
	if result[0].Content[0].OfText == nil {
		t.Fatal("expected first block to be text")
	}
	if result[0].Content[0].OfText.Text != "Let me check the weather." {
		t.Errorf("text = %q, want %q", result[0].Content[0].OfText.Text, "Let me check the weather.")
	}

	// Second block: tool_use
	if result[0].Content[1].OfToolUse == nil {
		t.Fatal("expected second block to be tool_use")
	}
	if result[0].Content[1].OfToolUse.ID != "toolu_abc123" {
		t.Errorf("tool_use ID = %q, want %q", result[0].Content[1].OfToolUse.ID, "toolu_abc123")
	}
	if result[0].Content[1].OfToolUse.Name != "get_weather" {
		t.Errorf("tool_use Name = %q, want %q", result[0].Content[1].OfToolUse.Name, "get_weather")
	}
}

func TestConvertMessages_ToolUseFollowedByToolResult(t *testing.T) {
	// Full round-trip: assistant tool_use + user tool_result
	// This is the exact scenario that was failing: cross-provider conversation history
	toolUseBlocks := []types.ContentBlock{
		{
			Type:  "tool_use",
			ID:    "toolu_01Mg8jVsVMP5nUi6RPogGSkk",
			Name:  "get_weather",
			Input: json.RawMessage(`{"location":"San Francisco"}`),
		},
	}
	toolUseJSON, _ := json.Marshal(toolUseBlocks)

	toolResultBlocks := []types.ContentBlock{
		{
			Type:      "tool_result",
			ToolUseID: "toolu_01Mg8jVsVMP5nUi6RPogGSkk",
			Content:   "72°F, sunny",
		},
	}
	toolResultJSON, _ := json.Marshal(toolResultBlocks)

	messages := []types.Message{
		{Role: "assistant", Content: toolUseJSON},
		{Role: "user", Content: toolResultJSON},
	}

	result, err := convertMessages(messages)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	// Assistant message should have tool_use block
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 block in assistant message, got %d", len(result[0].Content))
	}
	if result[0].Content[0].OfToolUse == nil {
		t.Fatal("expected assistant block to be tool_use")
	}
	if result[0].Content[0].OfToolUse.ID != "toolu_01Mg8jVsVMP5nUi6RPogGSkk" {
		t.Errorf("tool_use ID = %q, want %q", result[0].Content[0].OfToolUse.ID, "toolu_01Mg8jVsVMP5nUi6RPogGSkk")
	}

	// User message should have tool_result block
	if len(result[1].Content) != 1 {
		t.Fatalf("expected 1 block in user message, got %d", len(result[1].Content))
	}
	if result[1].Content[0].OfToolResult == nil {
		t.Fatal("expected user block to be tool_result")
	}
	if result[1].Content[0].OfToolResult.ToolUseID != "toolu_01Mg8jVsVMP5nUi6RPogGSkk" {
		t.Errorf("tool_result ToolUseID = %q, want %q", result[1].Content[0].OfToolResult.ToolUseID, "toolu_01Mg8jVsVMP5nUi6RPogGSkk")
	}
}

func TestConvertMessages_ToolUseNilInput(t *testing.T) {
	// Edge case: tool_use block with nil input
	blocks := []types.ContentBlock{
		{
			Type: "tool_use",
			ID:   "toolu_nil",
			Name: "no_args_tool",
		},
	}
	blocksJSON, _ := json.Marshal(blocks)

	messages := []types.Message{
		{Role: "assistant", Content: blocksJSON},
	}

	result, err := convertMessages(messages)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result[0].Content))
	}
	if result[0].Content[0].OfToolUse == nil {
		t.Fatal("expected tool_use block")
	}
	if result[0].Content[0].OfToolUse.Name != "no_args_tool" {
		t.Errorf("name = %q, want %q", result[0].Content[0].OfToolUse.Name, "no_args_tool")
	}
}

// ---------------------------------------------------------------------------
// processStreamEvents — streaming tool_use with input_json_delta
// ---------------------------------------------------------------------------

func TestProcessStreamEvents_ToolUseInputJSONDelta(t *testing.T) {
	// Regression test: input_json_delta events must read delta.PartialJSON,
	// not delta.Text. Previously delta.Text was used, which is always empty
	// for input_json_delta events, resulting in empty tool Input.

	dec := &mockDecoder{events: []ssestream.Event{
		makeEvent("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":    "msg_test123",
				"model": "claude-sonnet-4-20250514",
				"usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 0},
			},
		}),
		makeEvent("content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": 0,
			"content_block": map[string]interface{}{
				"type": "tool_use",
				"id":   "toolu_abc123",
				"name": "get_weather",
			},
		}),
		// Partial JSON arrives in multiple input_json_delta chunks
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type":         "input_json_delta",
				"partial_json": `{"locat`,
			},
		}),
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type":         "input_json_delta",
				"partial_json": `ion":"San`,
			},
		}),
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type":         "input_json_delta",
				"partial_json": ` Francisco"}`,
			},
		}),
		makeEvent("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": 0,
		}),
		makeEvent("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": "tool_use"},
			"usage": map[string]interface{}{"output_tokens": 5},
		}),
		makeEvent("message_stop", map[string]interface{}{
			"type": "message_stop",
		}),
	}}

	stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)
	events := make(chan types.StreamEvent, 20)

	processStreamEvents(stream, events)
	close(events)

	// Collect all events
	var allEvents []types.StreamEvent
	for ev := range events {
		allEvents = append(allEvents, ev)
	}

	// Find the ContentDone event with the tool_use block
	var toolBlock *types.ContentBlock
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventContentDone && ev.ContentBlock != nil {
			toolBlock = ev.ContentBlock
		}
	}

	if toolBlock == nil {
		t.Fatal("expected a ContentDone event with a tool_use block")
	}

	if toolBlock.Type != "tool_use" {
		t.Errorf("block type = %q, want %q", toolBlock.Type, "tool_use")
	}
	if toolBlock.ID != "toolu_abc123" {
		t.Errorf("block ID = %q, want %q", toolBlock.ID, "toolu_abc123")
	}
	if toolBlock.Name != "get_weather" {
		t.Errorf("block name = %q, want %q", toolBlock.Name, "get_weather")
	}

	// The critical assertion: Input must contain the accumulated JSON, not be empty.
	expectedInput := `{"location":"San Francisco"}`
	if string(toolBlock.Input) != expectedInput {
		t.Errorf("block Input = %q, want %q", string(toolBlock.Input), expectedInput)
	}

	// Also verify the Done event includes the tool block in the response
	var doneResp *types.CompletionResponse
	for _, ev := range allEvents {
		if ev.Type == types.StreamEventDone {
			doneResp = ev.Response
		}
	}
	if doneResp == nil {
		t.Fatal("expected a Done event with response")
	}
	if len(doneResp.Content) != 1 {
		t.Fatalf("expected 1 content block in response, got %d", len(doneResp.Content))
	}
	if string(doneResp.Content[0].Input) != expectedInput {
		t.Errorf("response content Input = %q, want %q", string(doneResp.Content[0].Input), expectedInput)
	}
}

func TestProcessStreamEvents_TextBlockInFullResponse(t *testing.T) {
	// Verify that text content blocks streamed via text_delta events
	// are accumulated and added to fullResponse.Content in the Done event.
	dec := &mockDecoder{events: []ssestream.Event{
		makeEvent("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":    "msg_textblock",
				"model": "claude-sonnet-4-20250514",
				"usage": map[string]interface{}{"input_tokens": 5, "output_tokens": 0},
			},
		}),
		makeEvent("content_block_start", map[string]interface{}{
			"type":          "content_block_start",
			"index":         0,
			"content_block": map[string]interface{}{"type": "text"},
		}),
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": "Hello ",
			},
		}),
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": "world!",
			},
		}),
		makeEvent("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": 0,
		}),
		makeEvent("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": "end_turn"},
			"usage": map[string]interface{}{"output_tokens": 2},
		}),
		makeEvent("message_stop", map[string]interface{}{
			"type": "message_stop",
		}),
	}}

	stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)
	events := make(chan types.StreamEvent, 20)

	processStreamEvents(stream, events)
	close(events)

	var doneResp *types.CompletionResponse
	for ev := range events {
		if ev.Type == types.StreamEventDone {
			doneResp = ev.Response
		}
	}

	if doneResp == nil {
		t.Fatal("expected a Done event with response")
	}
	if len(doneResp.Content) != 1 {
		t.Fatalf("expected 1 content block in response, got %d", len(doneResp.Content))
	}
	if doneResp.Content[0].Type != "text" {
		t.Errorf("content block type = %q, want %q", doneResp.Content[0].Type, "text")
	}
	if doneResp.Content[0].Text != "Hello world!" {
		t.Errorf("content block text = %q, want %q", doneResp.Content[0].Text, "Hello world!")
	}
}

func TestProcessStreamEvents_MixedTextAndToolUse(t *testing.T) {
	// Verify that a response with both a text block and a tool_use block
	// has both blocks present in fullResponse.Content.
	dec := &mockDecoder{events: []ssestream.Event{
		makeEvent("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":    "msg_mixed",
				"model": "claude-sonnet-4-20250514",
				"usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 0},
			},
		}),
		// First content block: text
		makeEvent("content_block_start", map[string]interface{}{
			"type":          "content_block_start",
			"index":         0,
			"content_block": map[string]interface{}{"type": "text"},
		}),
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": "Let me check the weather.",
			},
		}),
		makeEvent("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": 0,
		}),
		// Second content block: tool_use
		makeEvent("content_block_start", map[string]interface{}{
			"type":  "content_block_start",
			"index": 1,
			"content_block": map[string]interface{}{
				"type": "tool_use",
				"id":   "toolu_mixed123",
				"name": "get_weather",
			},
		}),
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 1,
			"delta": map[string]interface{}{
				"type":         "input_json_delta",
				"partial_json": `{"location":"Paris"}`,
			},
		}),
		makeEvent("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": 1,
		}),
		makeEvent("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": "tool_use"},
			"usage": map[string]interface{}{"output_tokens": 8},
		}),
		makeEvent("message_stop", map[string]interface{}{
			"type": "message_stop",
		}),
	}}

	stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)
	events := make(chan types.StreamEvent, 20)

	processStreamEvents(stream, events)
	close(events)

	var doneResp *types.CompletionResponse
	for ev := range events {
		if ev.Type == types.StreamEventDone {
			doneResp = ev.Response
		}
	}

	if doneResp == nil {
		t.Fatal("expected a Done event with response")
	}
	if len(doneResp.Content) != 2 {
		t.Fatalf("expected 2 content blocks in response, got %d", len(doneResp.Content))
	}

	// First block: text
	if doneResp.Content[0].Type != "text" {
		t.Errorf("first block type = %q, want %q", doneResp.Content[0].Type, "text")
	}
	if doneResp.Content[0].Text != "Let me check the weather." {
		t.Errorf("first block text = %q, want %q", doneResp.Content[0].Text, "Let me check the weather.")
	}

	// Second block: tool_use
	if doneResp.Content[1].Type != "tool_use" {
		t.Errorf("second block type = %q, want %q", doneResp.Content[1].Type, "tool_use")
	}
	if doneResp.Content[1].ID != "toolu_mixed123" {
		t.Errorf("second block ID = %q, want %q", doneResp.Content[1].ID, "toolu_mixed123")
	}
	if doneResp.Content[1].Name != "get_weather" {
		t.Errorf("second block name = %q, want %q", doneResp.Content[1].Name, "get_weather")
	}
	expectedInput := `{"location":"Paris"}`
	if string(doneResp.Content[1].Input) != expectedInput {
		t.Errorf("second block input = %q, want %q", string(doneResp.Content[1].Input), expectedInput)
	}
}

func TestProcessStreamEvents_TextDeltaUsesTextField(t *testing.T) {
	// Verify text_delta events correctly read the "text" field.
	dec := &mockDecoder{events: []ssestream.Event{
		makeEvent("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":    "msg_text",
				"model": "claude-sonnet-4-20250514",
				"usage": map[string]interface{}{"input_tokens": 5, "output_tokens": 0},
			},
		}),
		makeEvent("content_block_start", map[string]interface{}{
			"type":          "content_block_start",
			"index":         0,
			"content_block": map[string]interface{}{"type": "text"},
		}),
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": "Hello ",
			},
		}),
		makeEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": "world!",
			},
		}),
		makeEvent("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": 0,
		}),
		makeEvent("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": "end_turn"},
			"usage": map[string]interface{}{"output_tokens": 2},
		}),
		makeEvent("message_stop", map[string]interface{}{
			"type": "message_stop",
		}),
	}}

	stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)
	events := make(chan types.StreamEvent, 20)

	processStreamEvents(stream, events)
	close(events)

	var textContent string
	for ev := range events {
		if ev.Type == types.StreamEventDelta {
			textContent += ev.Content
		}
	}

	if textContent != "Hello world!" {
		t.Errorf("streamed text = %q, want %q", textContent, "Hello world!")
	}
}

// ---------------------------------------------------------------------------
// convertMessages — empty text block filtering
// ---------------------------------------------------------------------------

func TestConvertMessages_EmptyTextBlockSkipped(t *testing.T) {
	// An assistant message with content blocks that include an empty text block
	// should omit the empty text block from the Anthropic params.
	blocks := []types.ContentBlock{
		{Type: "text", Text: ""},
		{Type: "text", Text: "Hello"},
	}
	blocksJSON, _ := json.Marshal(blocks)

	messages := []types.Message{
		{Role: "assistant", Content: blocksJSON},
	}
	result, err := convertMessages(messages)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content) != 1 {
		t.Fatalf("expected 1 content block (empty text filtered), got %d", len(result[0].Content))
	}
	if result[0].Content[0].OfText == nil || result[0].Content[0].OfText.Text != "Hello" {
		t.Errorf("expected text block with 'Hello', got %+v", result[0].Content[0])
	}
}

func TestConvertMessages_AllEmptyBlocksOmitsMessage(t *testing.T) {
	// An assistant message with only empty text blocks should be omitted entirely.
	blocks := []types.ContentBlock{
		{Type: "text", Text: ""},
	}
	blocksJSON, _ := json.Marshal(blocks)

	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`"Hello"`)},
		{Role: "assistant", Content: blocksJSON},
		{Role: "user", Content: json.RawMessage(`"Follow up"`)},
	}
	result, err := convertMessages(messages)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	// The assistant message with all-empty blocks should be omitted.
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (assistant omitted), got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "user" {
		t.Errorf("expected user/user roles, got %s/%s", result[0].Role, result[1].Role)
	}
}

func TestConvertMessages_EmptyStringAssistantSkipped(t *testing.T) {
	// An assistant message with empty string content (from max_tokens truncation)
	// should be skipped to avoid sending {"type":"text","text":""}.
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`"Hello"`)},
		{Role: "assistant", Content: json.RawMessage(`""`)},
		{Role: "user", Content: json.RawMessage(`"Follow up"`)},
	}
	result, err := convertMessages(messages)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (empty assistant skipped), got %d", len(result))
	}
	if result[0].Role != "user" || result[1].Role != "user" {
		t.Errorf("expected user/user roles, got %s/%s", result[0].Role, result[1].Role)
	}
}

func TestConvertMessages_NonEmptyStringAssistantKept(t *testing.T) {
	// A normal assistant message with text should still be included.
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`"Hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"I can help."`)},
	}
	result, err := convertMessages(messages)
	if err != nil {
		t.Fatalf("convertMessages: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[1].Role != "assistant" {
		t.Errorf("expected assistant role, got %s", result[1].Role)
	}
}
