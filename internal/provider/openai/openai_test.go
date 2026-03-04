package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/langdag/langdag/pkg/types"
)

func TestConvertMessages_PlainText(t *testing.T) {
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`"Hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"Hi there"`)},
	}

	result := convertMessages(messages, "You are helpful")

	if len(result) != 3 {
		t.Fatalf("expected 3 messages (system + 2), got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("expected system role, got %s", result[0].Role)
	}
	if result[0].Content != "You are helpful" {
		t.Errorf("expected system content, got %v", result[0].Content)
	}
	if result[1].Role != "user" || result[1].Content != "Hello" {
		t.Errorf("unexpected user message: %+v", result[1])
	}
	if result[2].Role != "assistant" || result[2].Content != "Hi there" {
		t.Errorf("unexpected assistant message: %+v", result[2])
	}
}

func TestConvertMessages_ToolUse(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "text", Text: "Let me search"},
		{Type: "tool_use", ID: "call_1", Name: "search", Input: json.RawMessage(`{"q":"test"}`)},
	}
	content, _ := json.Marshal(blocks)

	messages := []types.Message{
		{Role: "assistant", Content: content},
	}

	result := convertMessages(messages, "")

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Role != "assistant" {
		t.Errorf("expected assistant role, got %s", result[0].Role)
	}
	if len(result[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result[0].ToolCalls))
	}
	if result[0].ToolCalls[0].Function.Name != "search" {
		t.Errorf("expected tool name 'search', got %s", result[0].ToolCalls[0].Function.Name)
	}
}

func TestConvertMessages_ToolResult(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "tool_result", ToolUseID: "call_1", Content: "result data"},
	}
	content, _ := json.Marshal(blocks)

	messages := []types.Message{
		{Role: "user", Content: content},
	}

	result := convertMessages(messages, "")

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Role != "tool" {
		t.Errorf("expected tool role, got %s", result[0].Role)
	}
	if result[0].ToolCallID != "call_1" {
		t.Errorf("expected tool_call_id 'call_1', got %s", result[0].ToolCallID)
	}
}

func TestMapUsage(t *testing.T) {
	u := &usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		PromptTokensDetails: &tokenDetails{
			CachedTokens: 30,
		},
		CompletionTokensDetails: &tokenDetails{
			ReasoningTokens: 10,
		},
	}

	result := mapUsage(u)

	if result.InputTokens != 100 {
		t.Errorf("expected InputTokens=100, got %d", result.InputTokens)
	}
	if result.OutputTokens != 50 {
		t.Errorf("expected OutputTokens=50, got %d", result.OutputTokens)
	}
	if result.CacheReadInputTokens != 30 {
		t.Errorf("expected CacheReadInputTokens=30, got %d", result.CacheReadInputTokens)
	}
	if result.ReasoningTokens != 10 {
		t.Errorf("expected ReasoningTokens=10, got %d", result.ReasoningTokens)
	}
}

func TestMapUsage_NoDetails(t *testing.T) {
	u := &usage{
		PromptTokens:     100,
		CompletionTokens: 50,
	}

	result := mapUsage(u)

	if result.CacheReadInputTokens != 0 {
		t.Errorf("expected CacheReadInputTokens=0, got %d", result.CacheReadInputTokens)
	}
	if result.ReasoningTokens != 0 {
		t.Errorf("expected ReasoningTokens=0, got %d", result.ReasoningTokens)
	}
}

func TestConvertResponse(t *testing.T) {
	stop := "stop"
	content := "Hello world"
	resp := &chatCompletionResponse{
		ID:    "chatcmpl-123",
		Model: "gpt-4o",
		Choices: []choice{
			{
				Message: responseMessage{
					Content: &content,
				},
				FinishReason: &stop,
			},
		},
		Usage: &usage{
			PromptTokens:     10,
			CompletionTokens: 5,
		},
	}

	result := convertResponse(resp)

	if result.ID != "chatcmpl-123" {
		t.Errorf("expected ID chatcmpl-123, got %s", result.ID)
	}
	if result.StopReason != "stop" {
		t.Errorf("expected stop reason 'stop', got %s", result.StopReason)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "Hello world" {
		t.Errorf("unexpected content: %+v", result.Content)
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("expected InputTokens=10, got %d", result.Usage.InputTokens)
	}
}

func TestConvertResponse_ToolCalls(t *testing.T) {
	stop := "tool_calls"
	resp := &chatCompletionResponse{
		ID:    "chatcmpl-456",
		Model: "gpt-4o",
		Choices: []choice{
			{
				Message: responseMessage{
					ToolCalls: []responseToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: responseFunction{
								Name:      "search",
								Arguments: `{"query":"test"}`,
							},
						},
					},
				},
				FinishReason: &stop,
			},
		},
		Usage: &usage{PromptTokens: 10, CompletionTokens: 5},
	}

	result := convertResponse(resp)

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Type != "tool_use" {
		t.Errorf("expected tool_use type, got %s", result.Content[0].Type)
	}
	if result.Content[0].Name != "search" {
		t.Errorf("expected name 'search', got %s", result.Content[0].Name)
	}
}

func TestParseSSEStream(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2}}

data: [DONE]

`

	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var collected []types.StreamEvent
	for e := range events {
		collected = append(collected, e)
	}

	// Should have: start, delta("Hello"), delta(" world"), done
	if len(collected) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(collected))
	}

	if collected[0].Type != types.StreamEventStart {
		t.Errorf("expected start event, got %s", collected[0].Type)
	}

	// Find text deltas
	var text string
	for _, e := range collected {
		if e.Type == types.StreamEventDelta {
			text += e.Content
		}
	}
	if text != "Hello world" {
		t.Errorf("expected 'Hello world', got '%s'", text)
	}

	// Last should be done with usage
	last := collected[len(collected)-1]
	if last.Type != types.StreamEventDone {
		t.Errorf("expected done event, got %s", last.Type)
	}
	if last.Response == nil {
		t.Fatal("expected response in done event")
	}
	if last.Response.Usage.InputTokens != 10 {
		t.Errorf("expected InputTokens=10, got %d", last.Response.Usage.InputTokens)
	}
}

func TestParseSSEStream_ToolCalls(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-2","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":""}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"test\"}"}}]},"finish_reason":null}]}

data: {"id":"chatcmpl-2","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"id":"chatcmpl-2","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":10}}

data: [DONE]

`

	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var collected []types.StreamEvent
	for e := range events {
		collected = append(collected, e)
	}

	// Find content_done event
	var foundToolDone bool
	for _, e := range collected {
		if e.Type == types.StreamEventContentDone {
			foundToolDone = true
			if e.ContentBlock.Name != "search" {
				t.Errorf("expected tool name 'search', got %s", e.ContentBlock.Name)
			}
			if string(e.ContentBlock.Input) != `{"q":"test"}` {
				t.Errorf("expected tool input '{\"q\":\"test\"}', got %s", string(e.ContentBlock.Input))
			}
		}
	}
	if !foundToolDone {
		t.Error("expected content_done event for tool call")
	}
}

func TestConvertTools(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "search",
			Description: "Search the web",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		},
	}

	result := convertTools(tools)

	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "function" {
		t.Errorf("expected type 'function', got %s", result[0].Type)
	}
	if result[0].Function.Name != "search" {
		t.Errorf("expected name 'search', got %s", result[0].Function.Name)
	}
}
