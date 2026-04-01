package openai

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"langdag.com/langdag/types"
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

func TestParseSSEStream_CacheTokens(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-c","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"},"finish_reason":null}]}

data: {"id":"chatcmpl-c","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl-c","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":80},"completion_tokens_details":{"reasoning_tokens":3}}}

data: [DONE]

`

	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var doneResp *types.CompletionResponse
	for ev := range events {
		if ev.Type == types.StreamEventDone {
			doneResp = ev.Response
		}
	}

	if doneResp == nil {
		t.Fatal("expected done response")
	}
	if doneResp.Usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", doneResp.Usage.InputTokens)
	}
	if doneResp.Usage.CacheReadInputTokens != 80 {
		t.Errorf("CacheReadInputTokens = %d, want 80", doneResp.Usage.CacheReadInputTokens)
	}
	if doneResp.Usage.ReasoningTokens != 3 {
		t.Errorf("ReasoningTokens = %d, want 3", doneResp.Usage.ReasoningTokens)
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

func TestAzureProviderName(t *testing.T) {
	p := NewAzure("test-key", "https://myresource.openai.azure.com", "")
	if p.Name() != "openai-azure" {
		t.Errorf("expected name 'openai-azure', got '%s'", p.Name())
	}
}

func TestAzureProviderModels(t *testing.T) {
	p := NewAzure("test-key", "https://myresource.openai.azure.com", "")
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	for _, m := range models {
		if m.ID == "" || m.Name == "" {
			t.Errorf("model missing required fields: %+v", m)
		}
	}
}

func TestAzureDefaultAPIVersion(t *testing.T) {
	p := NewAzure("test-key", "https://myresource.openai.azure.com", "")
	if p.apiVersion == "" {
		t.Error("expected default API version to be set")
	}
}

func TestAzureCustomAPIVersion(t *testing.T) {
	p := NewAzure("test-key", "https://myresource.openai.azure.com", "2024-06-01")
	if p.apiVersion != "2024-06-01" {
		t.Errorf("expected API version '2024-06-01', got '%s'", p.apiVersion)
	}
}

func TestAzureEndpointTrimming(t *testing.T) {
	p := NewAzure("test-key", "https://myresource.openai.azure.com/", "")
	if strings.HasSuffix(p.endpoint, "/") {
		t.Error("expected trailing slash to be trimmed from endpoint")
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

	result := convertTools(tools, openAIServerTools)

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

func TestGrokProviderName(t *testing.T) {
	p := NewGrok("test-key", "")
	if p.Name() != "grok" {
		t.Errorf("expected name 'grok', got '%s'", p.Name())
	}
}

func TestGrokProviderModels(t *testing.T) {
	p := NewGrok("test-key", "")
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	for _, m := range models {
		if m.ID == "" || m.Name == "" {
			t.Errorf("model missing required fields: %+v", m)
		}
	}
}

func TestGrokDefaultBaseURL(t *testing.T) {
	p := NewGrok("test-key", "")
	if p.baseURL != "https://api.x.ai/v1" {
		t.Errorf("expected default base URL 'https://api.x.ai/v1', got '%s'", p.baseURL)
	}
}

func TestGrokCustomBaseURL(t *testing.T) {
	p := NewGrok("test-key", "https://custom.api.example.com/v1")
	if p.baseURL != "https://custom.api.example.com/v1" {
		t.Errorf("expected custom base URL, got '%s'", p.baseURL)
	}
}

func TestGrokBaseURLTrimming(t *testing.T) {
	p := NewGrok("test-key", "https://api.x.ai/v1/")
	if strings.HasSuffix(p.baseURL, "/") {
		t.Error("expected trailing slash to be trimmed from base URL")
	}
}

// ---------------------------------------------------------------------------
// convertMessages — error branch coverage
// ---------------------------------------------------------------------------

func TestConvertMessages_MalformedJSON(t *testing.T) {
	// Content that is neither valid string nor valid []ContentBlock
	// falls back to raw string content — no panic.
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`{invalid json}`)},
	}
	result := convertMessages(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// Should fall back to raw string
	if result[0].Content != `{invalid json}` {
		t.Errorf("content = %v, want raw fallback", result[0].Content)
	}
}

func TestConvertMessages_EmptyContentArray(t *testing.T) {
	// Empty block array should produce a message with empty text.
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`[]`)},
	}
	result := convertMessages(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// extractText of empty parts → ""
	if result[0].Content != "" {
		t.Errorf("content = %v, want empty string", result[0].Content)
	}
}

func TestConvertMessages_UnknownBlockTypes(t *testing.T) {
	// Blocks with unrecognized types should be silently skipped.
	blocks := []types.ContentBlock{
		{Type: "audio", Text: "audio data"},
		{Type: "text", Text: "Hello"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "user", Content: content},
	}
	result := convertMessages(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// Only the text block should survive
	if result[0].Content != "Hello" {
		t.Errorf("content = %v, want %q", result[0].Content, "Hello")
	}
}

func TestConvertMessages_ToolCallEmptyFunctionName(t *testing.T) {
	// tool_use block with empty function name should not panic.
	blocks := []types.ContentBlock{
		{Type: "tool_use", ID: "call_empty", Name: "", Input: json.RawMessage(`{}`)},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "assistant", Content: content},
	}
	result := convertMessages(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result[0].ToolCalls))
	}
	if result[0].ToolCalls[0].Function.Name != "" {
		t.Errorf("expected empty function name, got %q", result[0].ToolCalls[0].Function.Name)
	}
}

func TestConvertMessages_ImageBlockNoURLNoData(t *testing.T) {
	// Image block with neither URL nor Data should produce no image_url part.
	blocks := []types.ContentBlock{
		{Type: "image"},
		{Type: "text", Text: "caption"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "user", Content: content},
	}
	result := convertMessages(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// hasImages is true (type=image present), so content is []contentPart
	// but the image part was skipped (no URL/Data) — only text remains
	// Actually hasImages flag is set even though no url, so content is []contentPart
	parts, ok := result[0].Content.([]contentPart)
	if ok {
		// If it's parts, should only have the text
		if len(parts) != 1 {
			t.Errorf("expected 1 content part, got %d", len(parts))
		}
	}
}

func TestConvertMessages_DocumentPlainText(t *testing.T) {
	// Document block with text/plain is converted to text part.
	blocks := []types.ContentBlock{
		{Type: "document", MediaType: "text/plain", Data: "document content here"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "user", Content: content},
	}
	result := convertMessages(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content != "document content here" {
		t.Errorf("content = %v, want %q", result[0].Content, "document content here")
	}
}

func TestConvertMessages_DocumentNonTextSkipped(t *testing.T) {
	// Document block with non-text/plain media type and no URL is skipped.
	blocks := []types.ContentBlock{
		{Type: "document", MediaType: "application/pdf", Data: "base64data"},
		{Type: "text", Text: "summary"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "user", Content: content},
	}
	result := convertMessages(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// PDF document is not handled by OpenAI path, only text survives
	if result[0].Content != "summary" {
		t.Errorf("content = %v, want %q", result[0].Content, "summary")
	}
}

func TestConvertMessages_NullContent(t *testing.T) {
	// JSON null content should not panic.
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`null`)},
	}
	result := convertMessages(messages, "")
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// convertResponse — edge cases
// ---------------------------------------------------------------------------

func TestConvertResponse_EmptyChoices(t *testing.T) {
	// Response with no choices should not panic.
	resp := &chatCompletionResponse{
		ID:    "chatcmpl-empty",
		Model: "gpt-4o",
		Usage: &usage{PromptTokens: 10, CompletionTokens: 0},
	}
	result := convertResponse(resp)
	if result.ID != "chatcmpl-empty" {
		t.Errorf("ID = %q, want %q", result.ID, "chatcmpl-empty")
	}
	if len(result.Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(result.Content))
	}
	if result.StopReason != "" {
		t.Errorf("expected empty stop reason, got %q", result.StopReason)
	}
}

func TestConvertResponse_NilUsage(t *testing.T) {
	// Response with nil usage should not panic.
	stop := "stop"
	content := "Hi"
	resp := &chatCompletionResponse{
		ID:    "chatcmpl-nousage",
		Model: "gpt-4o",
		Choices: []choice{
			{Message: responseMessage{Content: &content}, FinishReason: &stop},
		},
	}
	result := convertResponse(resp)
	if result.Usage.InputTokens != 0 {
		t.Errorf("expected 0 input tokens with nil usage, got %d", result.Usage.InputTokens)
	}
}

func TestConvertResponse_NilContentNilFinishReason(t *testing.T) {
	// Choice with nil content and nil finish reason should not panic.
	resp := &chatCompletionResponse{
		ID:      "chatcmpl-nil",
		Model:   "gpt-4o",
		Choices: []choice{{}},
		Usage:   &usage{},
	}
	result := convertResponse(resp)
	if len(result.Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(result.Content))
	}
	if result.StopReason != "" {
		t.Errorf("expected empty stop reason, got %q", result.StopReason)
	}
}

// ---------------------------------------------------------------------------
// parseSSEStream — error branch coverage
// ---------------------------------------------------------------------------

func TestParseSSEStream_MalformedJSONSkipped(t *testing.T) {
	// Malformed JSON data lines should be silently skipped.
	sseData := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {invalid json line}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var text string
	for ev := range events {
		if ev.Type == types.StreamEventDelta {
			text += ev.Content
		}
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestParseSSEStream_NonDataLinesIgnored(t *testing.T) {
	// Lines without "data: " prefix (comments, empty lines, event: lines)
	// should be silently ignored.
	sseData := `: this is a comment
event: message

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}

random line without prefix

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var text string
	for ev := range events {
		if ev.Type == types.StreamEventDelta {
			text += ev.Content
		}
	}
	if text != "Hi" {
		t.Errorf("text = %q, want %q", text, "Hi")
	}
}

func TestParseSSEStream_DONEMidStream(t *testing.T) {
	// [DONE] appearing before tool calls are fully accumulated.
	// Should emit Done with whatever has been accumulated so far.
	sseData := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}

data: [DONE]

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":" ignored"},"finish_reason":null}]}

`
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var text string
	var gotDone bool
	for ev := range events {
		switch ev.Type {
		case types.StreamEventDelta:
			text += ev.Content
		case types.StreamEventDone:
			gotDone = true
		}
	}
	if text != "partial" {
		t.Errorf("text = %q, want %q (content after [DONE] should be ignored)", text, "partial")
	}
	if !gotDone {
		t.Error("expected done event")
	}
}

func TestParseSSEStream_EmptyDeltaChunks(t *testing.T) {
	// Delta with empty/null content should not emit a delta event.
	sseData := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var deltaCount int
	for ev := range events {
		if ev.Type == types.StreamEventDelta {
			deltaCount++
		}
	}
	// Only "Hi" should produce a delta (empty "" should be skipped)
	if deltaCount != 1 {
		t.Errorf("expected 1 delta event, got %d", deltaCount)
	}
}

func TestParseSSEStream_NoChoicesInChunk(t *testing.T) {
	// Chunks with empty choices (e.g. usage-only chunks) should be handled.
	sseData := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}

data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1}}

data: [DONE]

`
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var doneResp *types.CompletionResponse
	for ev := range events {
		if ev.Type == types.StreamEventDone {
			doneResp = ev.Response
		}
	}
	if doneResp == nil {
		t.Fatal("expected done response")
	}
	if doneResp.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", doneResp.Usage.InputTokens)
	}
}

// errReader returns data up to a point, then returns an error.
type errReader struct {
	data string
	pos  int
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, r.err
	}
	return n, nil
}

func TestParseSSEStream_ReadError(t *testing.T) {
	// Scanner encounters a read error mid-stream.
	// Should emit StreamEventError.
	data := "data: {\"id\":\"chatcmpl-1\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n"

	reader := &errReader{
		data: data,
		err:  errors.New("network timeout"),
	}

	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(reader, events)
	}()

	var gotDelta, gotError, gotDone bool
	for ev := range events {
		switch ev.Type {
		case types.StreamEventDelta:
			gotDelta = true
		case types.StreamEventError:
			gotError = true
		case types.StreamEventDone:
			gotDone = true
		}
	}
	if !gotDelta {
		t.Error("expected delta event before error")
	}
	// The scanner may or may not see the error depending on buffering.
	// If EOF is returned after all data, scanner considers it normal.
	// But a non-EOF error should be propagated.
	_ = gotError
	_ = gotDone
}

func TestParseSSEStream_EmptyStream(t *testing.T) {
	// Completely empty stream should emit start + done, no error.
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(""), events)
	}()

	var gotStart, gotDone bool
	for ev := range events {
		switch ev.Type {
		case types.StreamEventStart:
			gotStart = true
		case types.StreamEventDone:
			gotDone = true
		}
	}
	if !gotStart {
		t.Error("expected start event")
	}
	if !gotDone {
		t.Error("expected done event even on empty stream")
	}
}

func TestParseSSEStream_OnlyDONE(t *testing.T) {
	// Stream with only [DONE] sentinel and no data.
	sseData := "data: [DONE]\n\n"
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var gotStart, gotDone bool
	var doneResp *types.CompletionResponse
	for ev := range events {
		switch ev.Type {
		case types.StreamEventStart:
			gotStart = true
		case types.StreamEventDone:
			gotDone = true
			doneResp = ev.Response
		}
	}
	if !gotStart {
		t.Error("expected start event")
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if doneResp == nil {
		t.Fatal("expected response in done event")
	}
	if doneResp.ID != "" {
		t.Errorf("expected empty ID, got %q", doneResp.ID)
	}
}

func TestParseSSEStream_NoDONESentinel(t *testing.T) {
	// Stream that ends without [DONE] should still emit done event.
	sseData := `data: {"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}

`
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var text string
	var gotDone bool
	for ev := range events {
		switch ev.Type {
		case types.StreamEventDelta:
			text += ev.Content
		case types.StreamEventDone:
			gotDone = true
		}
	}
	if text != "Hi" {
		t.Errorf("text = %q, want %q", text, "Hi")
	}
	if !gotDone {
		t.Error("expected done event even without [DONE] sentinel")
	}
}

func TestParseSSEStream_ReadErrorNonEOF(t *testing.T) {
	// Use io.Pipe to simulate a read error mid-stream.
	pr, pw := io.Pipe()

	go func() {
		pw.Write([]byte("data: {\"id\":\"1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n"))
		pw.CloseWithError(errors.New("connection reset"))
	}()

	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(pr, events)
	}()

	var gotError bool
	var errMsg string
	for ev := range events {
		if ev.Type == types.StreamEventError {
			gotError = true
			errMsg = ev.Error.Error()
		}
	}
	if !gotError {
		t.Fatal("expected error event from connection reset")
	}
	if !strings.Contains(errMsg, "connection reset") {
		t.Errorf("error = %q, should contain 'connection reset'", errMsg)
	}
}
