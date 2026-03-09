package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"langdag.com/langdag/types"
)

func TestBuildResponsesRequest_Basic(t *testing.T) {
	req := &types.CompletionRequest{
		Model:  "grok-3",
		System: "You are helpful.",
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		MaxTokens:   1024,
		Temperature: 0.7,
	}

	body := buildResponsesRequest(req, false)
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if m["model"] != "grok-3" {
		t.Errorf("model = %v, want %q", m["model"], "grok-3")
	}
	if m["instructions"] != "You are helpful." {
		t.Errorf("instructions = %v, want %q", m["instructions"], "You are helpful.")
	}
	if m["stream"] != false {
		t.Errorf("stream = %v, want false", m["stream"])
	}
	if m["store"] != false {
		t.Errorf("store = %v, want false", m["store"])
	}
	if m["max_output_tokens"] != float64(1024) {
		t.Errorf("max_output_tokens = %v, want 1024", m["max_output_tokens"])
	}

	input, ok := m["input"].([]interface{})
	if !ok || len(input) != 1 {
		t.Fatalf("expected 1 input item, got %v", m["input"])
	}
	msg := input[0].(map[string]interface{})
	if msg["role"] != "user" {
		t.Errorf("input[0].role = %v, want %q", msg["role"], "user")
	}
	if msg["content"] != "Hello" {
		t.Errorf("input[0].content = %v, want %q", msg["content"], "Hello")
	}
}

func TestBuildResponsesRequest_WithTools(t *testing.T) {
	req := &types.CompletionRequest{
		Model: "grok-3",
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"Search the web"`)},
		},
		Tools: []types.ToolDefinition{
			{Name: types.ServerToolWebSearch, Description: "Web search"},
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
			},
		},
	}

	body := buildResponsesRequest(req, true)
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if m["stream"] != true {
		t.Errorf("stream = %v, want true", m["stream"])
	}

	tools, ok := m["tools"].([]interface{})
	if !ok || len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %v", m["tools"])
	}

	// First tool: server-side web_search
	t0 := tools[0].(map[string]interface{})
	if t0["type"] != "web_search" {
		t.Errorf("tools[0].type = %v, want %q", t0["type"], "web_search")
	}

	// Second tool: function tool
	t1 := tools[1].(map[string]interface{})
	if t1["type"] != "function" {
		t.Errorf("tools[1].type = %v, want %q", t1["type"], "function")
	}
	if t1["name"] != "get_weather" {
		t.Errorf("tools[1].name = %v, want %q", t1["name"], "get_weather")
	}
}

func TestConvertResponsesMessages_ToolUseAndResult(t *testing.T) {
	toolUseBlocks := []types.ContentBlock{
		{Type: "text", Text: "Let me check."},
		{Type: "tool_use", ID: "call_123", Name: "get_weather", Input: json.RawMessage(`{"location":"SF"}`)},
	}
	toolUseJSON, _ := json.Marshal(toolUseBlocks)

	toolResultBlocks := []types.ContentBlock{
		{Type: "tool_result", ToolUseID: "call_123", Content: "72°F"},
	}
	toolResultJSON, _ := json.Marshal(toolResultBlocks)

	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`"What is the weather?"`)},
		{Role: "assistant", Content: toolUseJSON},
		{Role: "user", Content: toolResultJSON},
	}

	_, input := convertResponsesMessages(messages, "")
	if len(input) < 4 {
		t.Fatalf("expected at least 4 input items, got %d", len(input))
	}

	// Item 0: user message
	msg0, ok := input[0].(responsesInputMessage)
	if !ok || msg0.Role != "user" {
		t.Errorf("input[0] should be user message, got %T %+v", input[0], input[0])
	}

	// Item 1: assistant text
	msg1, ok := input[1].(responsesInputMessage)
	if !ok || msg1.Role != "assistant" {
		t.Errorf("input[1] should be assistant message, got %T %+v", input[1], input[1])
	}

	// Item 2: function call
	fc, ok := input[2].(responsesFunctionCallInput)
	if !ok {
		t.Fatalf("input[2] should be function call, got %T", input[2])
	}
	if fc.Type != "function_call" {
		t.Errorf("function_call.type = %q, want %q", fc.Type, "function_call")
	}
	if fc.CallID != "call_123" {
		t.Errorf("function_call.call_id = %q, want %q", fc.CallID, "call_123")
	}
	if fc.Name != "get_weather" {
		t.Errorf("function_call.name = %q, want %q", fc.Name, "get_weather")
	}

	// Item 3: function call output
	fco, ok := input[3].(responsesFunctionCallOutput)
	if !ok {
		t.Fatalf("input[3] should be function call output, got %T", input[3])
	}
	if fco.Type != "function_call_output" {
		t.Errorf("function_call_output.type = %q, want %q", fco.Type, "function_call_output")
	}
	if fco.CallID != "call_123" {
		t.Errorf("function_call_output.call_id = %q, want %q", fco.CallID, "call_123")
	}
	if fco.Output != "72°F" {
		t.Errorf("function_call_output.output = %q, want %q", fco.Output, "72°F")
	}
}

func TestConvertResponsesResult_TextOnly(t *testing.T) {
	resp := &responsesResponse{
		ID:    "resp_123",
		Model: "grok-3",
		Output: []responsesOutput{
			{
				Type: "message",
				Role: "assistant",
				Content: []responsesContentBlock{
					{Type: "output_text", Text: "Hello!"},
				},
			},
		},
		Usage: &responsesUsage{
			InputTokens:  10,
			OutputTokens: 5,
		},
		Status: "completed",
	}

	cr := convertResponsesResult(resp)
	if cr.ID != "resp_123" {
		t.Errorf("ID = %q, want %q", cr.ID, "resp_123")
	}
	if cr.Model != "grok-3" {
		t.Errorf("Model = %q, want %q", cr.Model, "grok-3")
	}
	if cr.StopReason != "stop" {
		t.Errorf("StopReason = %q, want %q", cr.StopReason, "stop")
	}
	if len(cr.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(cr.Content))
	}
	if cr.Content[0].Type != "text" {
		t.Errorf("content[0].type = %q, want %q", cr.Content[0].Type, "text")
	}
	if cr.Content[0].Text != "Hello!" {
		t.Errorf("content[0].text = %q, want %q", cr.Content[0].Text, "Hello!")
	}
	if cr.Usage.InputTokens != 10 || cr.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want in=10 out=5", cr.Usage)
	}
}

func TestConvertResponsesResult_WithFunctionCall(t *testing.T) {
	resp := &responsesResponse{
		ID:    "resp_456",
		Model: "grok-3",
		Output: []responsesOutput{
			{
				Type: "message",
				Role: "assistant",
				Content: []responsesContentBlock{
					{Type: "output_text", Text: "Let me check."},
				},
			},
			{
				Type:      "function_call",
				CallID:    "call_789",
				Name:      "get_weather",
				Arguments: `{"location":"SF"}`,
			},
		},
		Status: "completed",
	}

	cr := convertResponsesResult(resp)
	if cr.StopReason != "tool_calls" {
		t.Errorf("StopReason = %q, want %q", cr.StopReason, "tool_calls")
	}
	if len(cr.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(cr.Content))
	}
	if cr.Content[0].Type != "text" || cr.Content[0].Text != "Let me check." {
		t.Errorf("content[0] = %+v, want text block", cr.Content[0])
	}
	if cr.Content[1].Type != "tool_use" {
		t.Errorf("content[1].type = %q, want %q", cr.Content[1].Type, "tool_use")
	}
	if cr.Content[1].ID != "call_789" {
		t.Errorf("content[1].id = %q, want %q", cr.Content[1].ID, "call_789")
	}
	if cr.Content[1].Name != "get_weather" {
		t.Errorf("content[1].name = %q, want %q", cr.Content[1].Name, "get_weather")
	}
}

func TestParseResponsesSSEStream_TextOnly(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"grok-3","output":[],"status":"in_progress"}}`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","role":"assistant"}}`,
		`data: {"type":"response.content_part.added","output_index":0,"content_index":0}`,
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello"}`,
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":" world"}`,
		`data: {"type":"response.output_text.done","output_index":0,"content_index":0}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"grok-3","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello world"}]}],"usage":{"input_tokens":5,"output_tokens":2},"status":"completed"}}`,
	}, "\n")

	events := make(chan types.StreamEvent, 100)
	go func() {
		defer close(events)
		parseResponsesSSEStream(strings.NewReader(sse), events)
	}()

	var deltas []string
	var doneResp *types.CompletionResponse
	for ev := range events {
		switch ev.Type {
		case types.StreamEventDelta:
			deltas = append(deltas, ev.Content)
		case types.StreamEventDone:
			doneResp = ev.Response
		}
	}

	if got := strings.Join(deltas, ""); got != "Hello world" {
		t.Errorf("accumulated deltas = %q, want %q", got, "Hello world")
	}
	if doneResp == nil {
		t.Fatal("expected done response")
	}
	if doneResp.StopReason != "stop" {
		t.Errorf("stop reason = %q, want %q", doneResp.StopReason, "stop")
	}
	if doneResp.Usage.InputTokens != 5 || doneResp.Usage.OutputTokens != 2 {
		t.Errorf("usage = %+v, want in=5 out=2", doneResp.Usage)
	}
}

func TestParseResponsesSSEStream_FunctionCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_abc","name":"get_weather","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"loc"}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"ation\":\"SF\"}"}`,
		`data: {"type":"response.function_call_arguments.done","output_index":0}`,
		`data: {"type":"response.completed","response":{"id":"resp_2","model":"grok-3","output":[{"type":"function_call","call_id":"call_abc","name":"get_weather","arguments":"{\"location\":\"SF\"}"}],"usage":{"input_tokens":10,"output_tokens":8},"status":"completed"}}`,
	}, "\n")

	events := make(chan types.StreamEvent, 100)
	go func() {
		defer close(events)
		parseResponsesSSEStream(strings.NewReader(sse), events)
	}()

	var contentDone *types.ContentBlock
	var doneResp *types.CompletionResponse
	for ev := range events {
		switch ev.Type {
		case types.StreamEventContentDone:
			contentDone = ev.ContentBlock
		case types.StreamEventDone:
			doneResp = ev.Response
		}
	}

	if contentDone == nil {
		t.Fatal("expected content_done event with function call")
	}
	if contentDone.Type != "tool_use" {
		t.Errorf("content_done.type = %q, want %q", contentDone.Type, "tool_use")
	}
	if contentDone.ID != "call_abc" {
		t.Errorf("content_done.id = %q, want %q", contentDone.ID, "call_abc")
	}
	if contentDone.Name != "get_weather" {
		t.Errorf("content_done.name = %q, want %q", contentDone.Name, "get_weather")
	}
	if string(contentDone.Input) != `{"location":"SF"}` {
		t.Errorf("content_done.input = %s, want %s", string(contentDone.Input), `{"location":"SF"}`)
	}

	if doneResp == nil {
		t.Fatal("expected done response")
	}
	if doneResp.StopReason != "tool_calls" {
		t.Errorf("stop reason = %q, want %q", doneResp.StopReason, "tool_calls")
	}
}
