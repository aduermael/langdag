package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"langdag.com/langdag/types"
)

func TestConvertTools_FunctionOnly(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
		},
	}
	result := convertTools(tools, openAIServerTools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "function" {
		t.Errorf("type = %q, want %q", result[0].Type, "function")
	}
	if result[0].Function == nil {
		t.Fatal("expected Function to be set")
	}
	if result[0].Function.Name != "get_weather" {
		t.Errorf("function name = %q, want %q", result[0].Function.Name, "get_weather")
	}
}

func TestConvertTools_ServerToolWebSearch(t *testing.T) {
	tools := []types.ToolDefinition{
		{Name: types.ServerToolWebSearch},
	}
	result := convertTools(tools, openAIServerTools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "web_search_preview" {
		t.Errorf("type = %q, want %q", result[0].Type, "web_search_preview")
	}
	if result[0].Function != nil {
		t.Error("expected Function to be nil for server tool")
	}

	// Verify JSON serialization omits the function field
	b, err := json.Marshal(result[0])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if _, ok := m["function"]; ok {
		t.Errorf("expected 'function' key to be absent in JSON, got: %s", string(b))
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
	}
	result := convertTools(tools, openAIServerTools)
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}
	if result[0].Type != "function" || result[0].Function == nil {
		t.Error("expected first tool to be a function tool")
	}
	if result[1].Type != "web_search_preview" || result[1].Function != nil {
		t.Error("expected second tool to be web_search_preview server tool")
	}
}

func TestConvertTools_UnknownServerToolPassedThrough(t *testing.T) {
	tools := []types.ToolDefinition{
		{Name: "code_execution"},
	}
	result := convertTools(tools, openAIServerTools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "code_execution" {
		t.Errorf("type = %q, want %q", result[0].Type, "code_execution")
	}
	if result[0].Function != nil {
		t.Error("expected Function to be nil for unknown server tool")
	}
}

func TestConvertTools_NilMappingDropsServerTools(t *testing.T) {
	tools := []types.ToolDefinition{
		{Name: types.ServerToolWebSearch},
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}
	result := convertTools(tools, nil)
	if len(result) != 1 {
		t.Fatalf("expected one client tool, got %d: %+v", len(result), result)
	}
	if result[0].Type != "function" || result[0].Function == nil || result[0].Function.Name != "get_weather" {
		t.Fatalf("result = %+v, want get_weather function only", result)
	}
}

func TestConvertTools_WebSearchWithSchemaIsClientTool(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "web_search",
			Description: "Custom web search",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		},
	}
	result := convertTools(tools, openAIServerTools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "function" {
		t.Errorf("type = %q, want %q", result[0].Type, "function")
	}
	if result[0].Function == nil {
		t.Fatal("expected Function to be set for client tool")
	}
	if result[0].Function.Name != "web_search" {
		t.Errorf("function name = %q, want %q", result[0].Function.Name, "web_search")
	}
}

// --- Think field wiring tests ---

func TestBuildRequest_ThinkTrue(t *testing.T) {
	think := true
	req := &types.CompletionRequest{
		Model: "qwen3:8b",
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
		Think: &think,
	}
	body := buildRequest(req, false, nil)

	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	v, ok := m["think"]
	if !ok {
		t.Fatal("expected 'think' key in JSON")
	}
	if v != true {
		t.Errorf("think = %v, want true", v)
	}
}

func TestBuildOpenAIChatCompletionRequest_UsesOpenAIFields(t *testing.T) {
	think := true
	req := &types.CompletionRequest{
		Model:     "gpt-5.5-2026-04-23",
		MaxTokens: 123,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
		Think: &think,
	}

	body := buildOpenAIChatCompletionRequest(req, false, nil)
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := m["max_tokens"]; ok {
		t.Fatalf("request included max_tokens: %s", string(body))
	}
	if m["max_completion_tokens"] != float64(123) {
		t.Fatalf("max_completion_tokens = %v, want 123", m["max_completion_tokens"])
	}
	if _, ok := m["think"]; ok {
		t.Fatalf("request included think: %s", string(body))
	}
	if m["reasoning_effort"] != "medium" {
		t.Fatalf("reasoning_effort = %v, want medium", m["reasoning_effort"])
	}
}

func TestParseSSEStreamMapsLengthFinishReasonToMaxTokens(t *testing.T) {
	body := strings.NewReader(`data: {"id":"chatcmpl_1","model":"gpt-4.1","choices":[{"delta":{"content":"part"},"finish_reason":null}]}

data: {"id":"chatcmpl_1","model":"gpt-4.1","choices":[{"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}

data: [DONE]

`)
	events := make(chan types.StreamEvent, 8)
	parseSSEStream(body, events)
	close(events)

	var done *types.CompletionResponse
	for event := range events {
		if event.Type == types.StreamEventDone {
			done = event.Response
		}
	}
	if done == nil {
		t.Fatal("missing done event")
	}
	if done.StopReason != "max_tokens" {
		t.Fatalf("StopReason = %q, want max_tokens", done.StopReason)
	}
}

func TestBuildRequest_ThinkFalse(t *testing.T) {
	think := false
	req := &types.CompletionRequest{
		Model: "qwen3:8b",
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
		Think: &think,
	}
	body := buildRequest(req, false, nil)

	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	v, ok := m["think"]
	if !ok {
		t.Fatal("expected 'think' key in JSON when set to false")
	}
	if v != false {
		t.Errorf("think = %v, want false", v)
	}
}

func TestBuildRequest_ThinkNil(t *testing.T) {
	req := &types.CompletionRequest{
		Model: "gpt-4",
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}
	body := buildRequest(req, false, nil)

	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := m["think"]; ok {
		t.Errorf("expected 'think' key to be absent when nil, got JSON: %s", string(body))
	}
}

// --- Responses API tool conversion tests (used by Grok) ---

func TestConvertResponsesTools_ServerToolWebSearch(t *testing.T) {
	tools := []types.ToolDefinition{
		{Name: types.ServerToolWebSearch},
	}
	result := convertResponsesTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	b, _ := json.Marshal(result[0])
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if m["type"] != "web_search" {
		t.Errorf("type = %v, want %q", m["type"], "web_search")
	}
	// Should NOT have function-tool fields
	if _, ok := m["name"]; ok {
		t.Errorf("expected no 'name' key for server tool, got: %s", string(b))
	}
}

func TestConvertResponsesTools_FunctionTool(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
		},
	}
	result := convertResponsesTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	b, _ := json.Marshal(result[0])
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if m["type"] != "function" {
		t.Errorf("type = %v, want %q", m["type"], "function")
	}
	if m["name"] != "get_weather" {
		t.Errorf("name = %v, want %q", m["name"], "get_weather")
	}
	if m["description"] != "Get weather" {
		t.Errorf("description = %v, want %q", m["description"], "Get weather")
	}
}

func TestConvertResponsesTools_Mixed(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
		{Name: types.ServerToolWebSearch},
	}
	result := convertResponsesTools(tools)
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}

	b0, _ := json.Marshal(result[0])
	var m0 map[string]interface{}
	json.Unmarshal(b0, &m0)
	if m0["type"] != "function" {
		t.Errorf("first tool type = %v, want %q", m0["type"], "function")
	}

	b1, _ := json.Marshal(result[1])
	var m1 map[string]interface{}
	json.Unmarshal(b1, &m1)
	if m1["type"] != "web_search" {
		t.Errorf("second tool type = %v, want %q", m1["type"], "web_search")
	}
}

func TestConvertResponsesTools_WebSearchWithSchemaIsClientTool(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "web_search",
			Description: "Custom web search",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		},
	}
	result := convertResponsesTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	b, _ := json.Marshal(result[0])
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if m["type"] != "function" {
		t.Errorf("type = %v, want %q", m["type"], "function")
	}
	if m["name"] != "web_search" {
		t.Errorf("name = %v, want %q", m["name"], "web_search")
	}
}
