package gemini

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

	result := convertMessages(messages)

	if len(result) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected user role, got %s", result[0].Role)
	}
	if result[1].Role != "model" {
		t.Errorf("expected model role for assistant, got %s", result[1].Role)
	}
	if result[0].Parts[0].Text != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", result[0].Parts[0].Text)
	}
}

func TestConvertMessages_ToolUse(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "text", Text: "Let me search"},
		{Type: "tool_use", Name: "search", Input: json.RawMessage(`{"q":"test"}`)},
	}
	content, _ := json.Marshal(blocks)

	messages := []types.Message{
		{Role: "assistant", Content: content},
	}

	result := convertMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
	if result[0].Role != "model" {
		t.Errorf("expected model role, got %s", result[0].Role)
	}
	if len(result[0].Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(result[0].Parts))
	}
	if result[0].Parts[0].Text != "Let me search" {
		t.Errorf("expected text part, got %+v", result[0].Parts[0])
	}
	if result[0].Parts[1].FunctionCall == nil {
		t.Fatal("expected function call part")
	}
	if result[0].Parts[1].FunctionCall.Name != "search" {
		t.Errorf("expected function name 'search', got %s", result[0].Parts[1].FunctionCall.Name)
	}
}

func TestConvertMessages_ToolResult(t *testing.T) {
	blocks := []types.ContentBlock{
		{Type: "tool_result", ToolUseID: "search", Content: "result data"},
	}
	content, _ := json.Marshal(blocks)

	messages := []types.Message{
		{Role: "user", Content: content},
	}

	result := convertMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
	if result[0].Parts[0].FunctionResponse == nil {
		t.Fatal("expected function response part")
	}
	if result[0].Parts[0].FunctionResponse.Name != "search" {
		t.Errorf("expected name 'search', got %s", result[0].Parts[0].FunctionResponse.Name)
	}
}

func TestMapUsage(t *testing.T) {
	u := &usageMetadata{
		PromptTokenCount:        100,
		CandidatesTokenCount:    50,
		CachedContentTokenCount: 30,
		ThoughtsTokenCount:      10,
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

func TestConvertResponse(t *testing.T) {
	resp := &geminiResponse{
		Candidates: []candidate{
			{
				Content: content{
					Parts: []part{
						{Text: "Hello world"},
					},
				},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: &usageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
		},
	}

	result := convertResponse(resp)

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

func TestConvertResponse_FunctionCall(t *testing.T) {
	resp := &geminiResponse{
		Candidates: []candidate{
			{
				Content: content{
					Parts: []part{
						{FunctionCall: &functionCall{
							Name: "search",
							Args: map[string]interface{}{"q": "test"},
						}},
					},
				},
			},
		},
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
	if result.Content[0].ID != "search" {
		t.Errorf("expected ID 'search', got %s", result.Content[0].ID)
	}
}

func TestConvertResponse_ToolUseID(t *testing.T) {
	// Regression test: Gemini doesn't provide unique tool call IDs, so the
	// adapter must set ID = FunctionCall.Name. Without this, downstream
	// tool_result blocks send function_response with Name="" and Gemini
	// returns 400 INVALID_ARGUMENT.
	resp := &geminiResponse{
		Candidates: []candidate{{
			Content: content{
				Parts: []part{
					{Text: "I'll search for that"},
					{FunctionCall: &functionCall{
						Name: "get_weather",
						Args: map[string]interface{}{"location": "Paris"},
					}},
					{FunctionCall: &functionCall{
						Name: "get_time",
						Args: map[string]interface{}{"timezone": "CET"},
					}},
				},
			},
		}},
	}

	result := convertResponse(resp)

	if len(result.Content) != 3 {
		t.Fatalf("expected 3 content blocks, got %d", len(result.Content))
	}

	// Text block
	if result.Content[0].Type != "text" {
		t.Errorf("expected text block, got %s", result.Content[0].Type)
	}

	// First tool_use
	if result.Content[1].ID == "" {
		t.Error("tool_use block ID should not be empty")
	}
	if result.Content[1].ID != "get_weather" {
		t.Errorf("expected ID 'get_weather', got %s", result.Content[1].ID)
	}
	if result.Content[1].Name != "get_weather" {
		t.Errorf("expected Name 'get_weather', got %s", result.Content[1].Name)
	}

	// Second tool_use
	if result.Content[2].ID == "" {
		t.Error("tool_use block ID should not be empty")
	}
	if result.Content[2].ID != "get_time" {
		t.Errorf("expected ID 'get_time', got %s", result.Content[2].ID)
	}
	if result.Content[2].Name != "get_time" {
		t.Errorf("expected Name 'get_time', got %s", result.Content[2].Name)
	}
}

func TestParseSSEStream(t *testing.T) {
	// Gemini sends full response snapshots
	sseData := `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1}}

data: {"candidates":[{"content":{"parts":[{"text":"Hello world"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}}

data: {"candidates":[{"content":{"parts":[{"text":"Hello world!"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3}}

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

	if collected[0].Type != types.StreamEventStart {
		t.Errorf("expected start event, got %s", collected[0].Type)
	}

	// Collect text deltas
	var text string
	for _, e := range collected {
		if e.Type == types.StreamEventDelta {
			text += e.Content
		}
	}
	if text != "Hello world!" {
		t.Errorf("expected 'Hello world!', got '%s'", text)
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
	if last.Response.StopReason != "stop" {
		t.Errorf("expected StopReason='stop', got %q", last.Response.StopReason)
	}
}

func TestParseSSEStream_MaxTokensFinishReason(t *testing.T) {
	// When the model hits max_tokens, the finishReason should be captured
	// so downstream continuation logic can detect the truncation.
	sseData := `data: {"candidates":[{"content":{"parts":[{"text":"Hello! How can I help you with your"}]},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":12,"thoughtsTokenCount":28}}

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
	if doneResp.StopReason != "max_tokens" {
		t.Errorf("StopReason = %q, want %q", doneResp.StopReason, "max_tokens")
	}
	if len(doneResp.Content) != 1 || doneResp.Content[0].Text != "Hello! How can I help you with your" {
		t.Errorf("unexpected content: %+v", doneResp.Content)
	}
}

func TestParseSSEStream_CacheTokens(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"parts":[{"text":"Hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":5,"cachedContentTokenCount":80,"thoughtsTokenCount":3}}

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

func TestParseSSEStream_DoneResponseContainsText(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}

data: {"candidates":[{"content":{"parts":[{"text":"Hello world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2}}

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

	// Find the done event
	var done *types.StreamEvent
	for i := range collected {
		if collected[i].Type == types.StreamEventDone {
			done = &collected[i]
			break
		}
	}
	if done == nil {
		t.Fatal("expected a done event")
	}
	if done.Response == nil {
		t.Fatal("expected response in done event")
	}
	if len(done.Response.Content) != 1 {
		t.Fatalf("expected 1 content block in done response, got %d", len(done.Response.Content))
	}
	if done.Response.Content[0].Type != "text" {
		t.Errorf("expected text block, got %s", done.Response.Content[0].Type)
	}
	if done.Response.Content[0].Text != "Hello world" {
		t.Errorf("expected full text 'Hello world', got '%s'", done.Response.Content[0].Text)
	}
}

func TestParseSSEStream_DoneResponseContainsFunctionCall(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"search","args":{"q":"test"}}}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":5}}

data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"search","args":{"q":"test"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":5}}

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

	// Find the done event
	var done *types.StreamEvent
	for i := range collected {
		if collected[i].Type == types.StreamEventDone {
			done = &collected[i]
			break
		}
	}
	if done == nil {
		t.Fatal("expected a done event")
	}
	if done.Response == nil {
		t.Fatal("expected response in done event")
	}
	// Should have at least one tool_use block (may be duplicated from multiple SSE chunks)
	hasToolUse := false
	for _, block := range done.Response.Content {
		if block.Type == "tool_use" && block.Name == "search" {
			hasToolUse = true
			if block.ID != "search" {
				t.Errorf("expected ID 'search' in done response, got %s", block.ID)
			}
			var args map[string]interface{}
			if err := json.Unmarshal(block.Input, &args); err != nil {
				t.Fatalf("failed to unmarshal tool input: %v", err)
			}
			if args["q"] != "test" {
				t.Errorf("expected args.q='test', got %v", args["q"])
			}
			break
		}
	}
	if !hasToolUse {
		t.Errorf("expected at least one tool_use block in done response, got content: %+v", done.Response.Content)
	}

	// ContentDone events should also have ID set
	for _, e := range collected {
		if e.Type == types.StreamEventContentDone && e.ContentBlock != nil && e.ContentBlock.Type == "tool_use" {
			if e.ContentBlock.ID != "search" {
				t.Errorf("expected ContentDone block ID 'search', got %s", e.ContentBlock.ID)
			}
		}
	}
}

func TestParseSSEStream_ToolUseID(t *testing.T) {
	// Regression test: streamed ContentDone blocks and the final Done response
	// must contain a non-empty ID equal to the function name, otherwise Gemini
	// rejects downstream function_response payloads with 400 INVALID_ARGUMENT.
	sseData := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"location":"Paris"}}}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3}}

data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"location":"Paris"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3}}

`

	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var contentDoneBlocks []*types.ContentBlock
	var doneResp *types.CompletionResponse
	for e := range events {
		if e.Type == types.StreamEventContentDone && e.ContentBlock != nil {
			contentDoneBlocks = append(contentDoneBlocks, e.ContentBlock)
		}
		if e.Type == types.StreamEventDone {
			doneResp = e.Response
		}
	}

	// Verify ContentDone events
	if len(contentDoneBlocks) == 0 {
		t.Fatal("expected at least one ContentDone event with tool_use block")
	}
	for i, block := range contentDoneBlocks {
		if block.ID == "" {
			t.Errorf("ContentDone block[%d]: ID should not be empty", i)
		}
		if block.ID != "get_weather" {
			t.Errorf("ContentDone block[%d]: expected ID 'get_weather', got %s", i, block.ID)
		}
		if block.Name != "get_weather" {
			t.Errorf("ContentDone block[%d]: expected Name 'get_weather', got %s", i, block.Name)
		}
	}

	// Verify Done response
	if doneResp == nil {
		t.Fatal("expected done response")
	}
	hasToolUse := false
	for _, block := range doneResp.Content {
		if block.Type == "tool_use" {
			hasToolUse = true
			if block.ID == "" {
				t.Error("Done response tool_use block ID should not be empty")
			}
			if block.ID != "get_weather" {
				t.Errorf("Done response: expected ID 'get_weather', got %s", block.ID)
			}
		}
	}
	if !hasToolUse {
		t.Error("expected tool_use block in Done response")
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
		t.Fatalf("expected 1 tool group, got %d", len(result))
	}
	if len(result[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(result[0].FunctionDeclarations))
	}
	if result[0].FunctionDeclarations[0].Name != "search" {
		t.Errorf("expected name 'search', got %s", result[0].FunctionDeclarations[0].Name)
	}
}

func TestProviderModelsIncludesGemma(t *testing.T) {
	p := New("test-key")
	models := p.Models()

	want := map[string]bool{
		"gemma-4-31b-it":     false,
		"gemma-4-26b-a4b-it": false,
		"gemma-3-27b-it":     false,
		"gemma-3-12b-it":     false,
		"gemma-3-4b-it":      false,
		"gemma-3-1b-it":      false,
	}
	for _, m := range models {
		if _, ok := want[m.ID]; ok {
			want[m.ID] = true
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("expected model %q in catalog", id)
		}
	}
}

func TestVertexProviderName(t *testing.T) {
	p := &VertexProvider{}
	if p.Name() != "gemini-vertex" {
		t.Errorf("expected name 'gemini-vertex', got '%s'", p.Name())
	}
}

func TestVertexProviderModels(t *testing.T) {
	p := &VertexProvider{}
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

func TestBuildRequest_SystemInstruction(t *testing.T) {
	req := &types.CompletionRequest{
		Model:     "gemini-3-flash-preview",
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
		System:    "You are helpful",
		MaxTokens: 1000,
	}

	body, err := buildRequest(req)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	var gr geminiRequest
	json.Unmarshal(body, &gr)

	if gr.SystemInstruction == nil {
		t.Fatal("expected system instruction")
	}
	if gr.SystemInstruction.Parts[0].Text != "You are helpful" {
		t.Errorf("expected system text, got %s", gr.SystemInstruction.Parts[0].Text)
	}
	if gr.GenerationConfig == nil || gr.GenerationConfig.MaxOutputTokens != 1000 {
		t.Errorf("expected max_output_tokens=1000")
	}
}

func TestBuildRequest_ThinkTrue(t *testing.T) {
	thinkTrue := true
	req := &types.CompletionRequest{
		Model:    "gemini-3-flash-preview",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
		Think:    &thinkTrue,
	}

	body, err := buildRequest(req)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	// Verify via struct unmarshaling
	var gr geminiRequest
	if err := json.Unmarshal(body, &gr); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if gr.GenerationConfig == nil {
		t.Fatal("expected generation_config to be set")
	}
	if gr.GenerationConfig.ThinkingConfig == nil {
		t.Fatal("expected thinkingConfig to be set")
	}
	if gr.GenerationConfig.ThinkingConfig.ThinkingBudget != 8192 {
		t.Errorf("expected thinkingBudget=8192, got %d", gr.GenerationConfig.ThinkingConfig.ThinkingBudget)
	}

	// Verify thinkingBudget appears in serialized JSON
	if !strings.Contains(string(body), `"thinkingBudget"`) {
		t.Errorf("expected thinkingBudget in JSON, got: %s", string(body))
	}
}

func TestBuildRequest_ThinkFalse(t *testing.T) {
	thinkFalse := false
	req := &types.CompletionRequest{
		Model:    "gemini-3-flash-preview",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
		Think:    &thinkFalse,
	}

	body, err := buildRequest(req)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	// Verify via struct unmarshaling
	var gr geminiRequest
	if err := json.Unmarshal(body, &gr); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if gr.GenerationConfig == nil {
		t.Fatal("expected generation_config to be set")
	}
	if gr.GenerationConfig.ThinkingConfig == nil {
		t.Fatal("expected thinkingConfig to be set when Think=false")
	}
	if gr.GenerationConfig.ThinkingConfig.ThinkingBudget != 0 {
		t.Errorf("expected thinkingBudget=0, got %d", gr.GenerationConfig.ThinkingConfig.ThinkingBudget)
	}

	// Verify thinkingBudget:0 appears in serialized JSON
	if !strings.Contains(string(body), `"thinkingBudget":0`) {
		t.Errorf("expected thinkingBudget:0 in JSON, got: %s", string(body))
	}
}

func TestBuildRequest_ThinkNil(t *testing.T) {
	req := &types.CompletionRequest{
		Model:    "gemini-3-flash-preview",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
		// Think is nil (not set)
	}

	body, err := buildRequest(req)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	// Verify thinkingConfig is NOT in serialized JSON
	if strings.Contains(string(body), `"thinkingConfig"`) {
		t.Errorf("expected no thinkingConfig in JSON when Think is nil, got: %s", string(body))
	}

	// Also verify no generation_config at all (no other config fields set)
	if strings.Contains(string(body), `"generation_config"`) {
		t.Errorf("expected no generation_config when no config fields set, got: %s", string(body))
	}
}

// ---------------------------------------------------------------------------
// featureCheck — fail-closed per-model capability enforcement
// ---------------------------------------------------------------------------

// TestFeatureCheck_RefusesUnsupported covers the three axes the caps table
// enforces. Each case is a (model, request) pair that should be refused with a
// typed *ErrFeatureUnsupported carrying the expected Feature tag.
func TestFeatureCheck_RefusesUnsupported(t *testing.T) {
	toolReq := &types.CompletionRequest{
		Model:    "gemma-3-1b-it",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools: []types.ToolDefinition{
			{Name: "x", Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	searchReq := &types.CompletionRequest{
		Model:    "gemma-3-1b-it",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools:    []types.ToolDefinition{{Name: types.ServerToolWebSearch}},
	}
	think := true
	thinkReq := &types.CompletionRequest{
		Model:    "gemma-4-26b-a4b-it",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Think:    &think,
	}

	cases := []struct {
		name    string
		req     *types.CompletionRequest
		feature string
	}{
		{"function_calling_on_gemma_3_1b", toolReq, "function_calling"},
		{"google_search_on_gemma_3_1b", searchReq, "google_search"},
		{"explicit_thinking_on_gemma_4", thinkReq, "explicit_thinking_budget"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildRequest(tc.req)
			var fe *ErrFeatureUnsupported
			if !errors.As(err, &fe) {
				t.Fatalf("expected *ErrFeatureUnsupported, got: %v", err)
			}
			if fe.Feature != tc.feature {
				t.Errorf("expected Feature=%q, got %q", tc.feature, fe.Feature)
			}
			if fe.Model != tc.req.Model {
				t.Errorf("expected Model=%q, got %q", tc.req.Model, fe.Model)
			}
		})
	}
}

// TestFeatureCheck_AllowsSupported verifies the caps table permits requests
// when the target model supports the requested feature (no false-positives).
func TestFeatureCheck_AllowsSupported(t *testing.T) {
	think := true
	req := &types.CompletionRequest{
		Model:    "gemini-3-flash-preview",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools: []types.ToolDefinition{
			{Name: "x", Description: "d", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: types.ServerToolWebSearch},
		},
		Think: &think,
	}
	if _, err := buildRequest(req); err != nil {
		t.Fatalf("expected success on fully-capable model, got: %v", err)
	}
}

// TestFeatureCheck_UnknownModelPermissive verifies the caps table is
// permissive for unknown models — the API becomes the arbiter.
func TestFeatureCheck_UnknownModelPermissive(t *testing.T) {
	think := true
	req := &types.CompletionRequest{
		Model:    "gemini-99-unreleased",
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Think:    &think,
	}
	if _, err := buildRequest(req); err != nil {
		t.Fatalf("expected permissive pass-through for unknown model, got: %v", err)
	}
}

// TestModels_NoSilentDrift guards against the silent inconsistency that would
// arise if a future model is added to (Provider).Models() or
// (VertexProvider).Models() without a matching modelCaps entry. applyCaps
// would return zero-value caps (all false) so ModelInfo would claim
// unsupported, while featureCheck would fall through to the permissive
// unknown-model branch and let requests pass — the public surface and the
// request path would disagree.
func TestModels_NoSilentDrift(t *testing.T) {
	check := func(t *testing.T, providerName string, models []types.ModelInfo) {
		t.Helper()
		for _, m := range models {
			if _, ok := modelCaps[m.ID]; !ok {
				t.Errorf("%s.Models() returns %q which is missing from modelCaps "+
					"— add a row or remove the model", providerName, m.ID)
			}
		}
	}
	check(t, "Provider", New("dummy").Models())
	// VertexProvider.Models() does not access instance state, so a
	// zero-value receiver is enough — keeps the test offline.
	check(t, "VertexProvider", (&VertexProvider{}).Models())
}

// TestModels_CapFieldsPopulated verifies that per-model cap fields flow from
// modelCaps into ModelInfo. The flat ServerTools list on every model was a
// known lie pre-refactor; this pins the correct per-model derivation.
func TestModels_CapFieldsPopulated(t *testing.T) {
	p := New("dummy")
	models := p.Models()
	byID := map[string]types.ModelInfo{}
	for _, m := range models {
		byID[m.ID] = m
	}

	cases := []struct {
		id                 string
		wantFunction       bool
		wantThinkingBudget bool
		wantSearchTool     bool
	}{
		{"gemini-3-flash-preview", true, true, true},
		{"gemma-4-26b-a4b-it", true, false, true},
		{"gemma-3-1b-it", false, false, false},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			m, ok := byID[c.id]
			if !ok {
				t.Fatalf("model %q not in Models()", c.id)
			}
			if m.SupportsFunctionCalling != c.wantFunction {
				t.Errorf("SupportsFunctionCalling: got %v want %v", m.SupportsFunctionCalling, c.wantFunction)
			}
			if m.SupportsExplicitThinkingBudget != c.wantThinkingBudget {
				t.Errorf("SupportsExplicitThinkingBudget: got %v want %v", m.SupportsExplicitThinkingBudget, c.wantThinkingBudget)
			}
			hasSearch := false
			for _, st := range m.ServerTools {
				if st == types.ServerToolWebSearch {
					hasSearch = true
					break
				}
			}
			if hasSearch != c.wantSearchTool {
				t.Errorf("ServerTools contains ServerToolWebSearch: got %v want %v (tools=%v)", hasSearch, c.wantSearchTool, m.ServerTools)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// convertMessages — error branch coverage
// ---------------------------------------------------------------------------

func TestConvertMessages_MalformedJSON(t *testing.T) {
	// Content that is neither valid string nor valid []ContentBlock
	// falls back to raw string — no panic.
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`{invalid}`)},
	}
	result := convertMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
	if result[0].Parts[0].Text != `{invalid}` {
		t.Errorf("expected raw fallback text, got %q", result[0].Parts[0].Text)
	}
}

func TestConvertMessages_EmptyContentArray(t *testing.T) {
	// Empty block array → no parts → message skipped.
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`[]`)},
	}
	result := convertMessages(messages)
	if len(result) != 0 {
		t.Errorf("expected 0 contents (empty parts skipped), got %d", len(result))
	}
}

func TestConvertMessages_UnknownBlockTypes(t *testing.T) {
	// Unknown block types should be silently skipped.
	blocks := []types.ContentBlock{
		{Type: "audio", Text: "audio data"},
		{Type: "text", Text: "Hello"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "user", Content: content},
	}
	result := convertMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
	if len(result[0].Parts) != 1 {
		t.Fatalf("expected 1 part (audio skipped), got %d", len(result[0].Parts))
	}
	if result[0].Parts[0].Text != "Hello" {
		t.Errorf("text = %q, want %q", result[0].Parts[0].Text, "Hello")
	}
}

func TestConvertMessages_FunctionCallNilArgs(t *testing.T) {
	// tool_use with nil Input → args is nil → FunctionCall.Args is nil.
	blocks := []types.ContentBlock{
		{Type: "tool_use", Name: "no_args"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "assistant", Content: content},
	}
	result := convertMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
	fc := result[0].Parts[0].FunctionCall
	if fc == nil {
		t.Fatal("expected FunctionCall part")
	}
	if fc.Name != "no_args" {
		t.Errorf("name = %q, want %q", fc.Name, "no_args")
	}
	if fc.Args != nil {
		t.Errorf("expected nil Args, got %v", fc.Args)
	}
}

func TestConvertMessages_FunctionResponseNonJSON(t *testing.T) {
	// tool_result content is always wrapped as {"result": content}.
	blocks := []types.ContentBlock{
		{Type: "tool_result", ToolUseID: "fn", Content: "plain text result"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "user", Content: content},
	}
	result := convertMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
	fr := result[0].Parts[0].FunctionResponse
	if fr == nil {
		t.Fatal("expected FunctionResponse part")
	}
	if fr.Name != "fn" {
		t.Errorf("name = %q, want %q", fr.Name, "fn")
	}
	if fr.Response["result"] != "plain text result" {
		t.Errorf("response = %v, want {result: plain text result}", fr.Response)
	}
}

func TestConvertMessages_ImageNoDataNoURL(t *testing.T) {
	// Image block with neither Data nor URL should produce no part.
	blocks := []types.ContentBlock{
		{Type: "image", MediaType: "image/png"},
		{Type: "text", Text: "fallback"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "user", Content: content},
	}
	result := convertMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
	if len(result[0].Parts) != 1 {
		t.Fatalf("expected 1 part (empty image skipped), got %d", len(result[0].Parts))
	}
	if result[0].Parts[0].Text != "fallback" {
		t.Errorf("text = %q, want %q", result[0].Parts[0].Text, "fallback")
	}
}

func TestConvertMessages_NullContent(t *testing.T) {
	// null JSON content should not panic.
	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(`null`)},
	}
	result := convertMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
}

func TestConvertMessages_AllUnknownBlocksSkipsMessage(t *testing.T) {
	// If all blocks are unknown, parts is empty → message skipped.
	blocks := []types.ContentBlock{
		{Type: "audio"},
		{Type: "video"},
	}
	content, _ := json.Marshal(blocks)
	messages := []types.Message{
		{Role: "user", Content: content},
	}
	result := convertMessages(messages)
	if len(result) != 0 {
		t.Errorf("expected 0 contents (all unknown blocks), got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// convertResponse — error branch coverage
// ---------------------------------------------------------------------------

func TestConvertResponse_NoCandidates(t *testing.T) {
	resp := &geminiResponse{
		UsageMetadata: &usageMetadata{PromptTokenCount: 10},
	}
	result := convertResponse(resp)
	if len(result.Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(result.Content))
	}
	if result.StopReason != "" {
		t.Errorf("expected empty stop reason, got %q", result.StopReason)
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", result.Usage.InputTokens)
	}
}

func TestConvertResponse_EmptyContent(t *testing.T) {
	resp := &geminiResponse{
		Candidates: []candidate{
			{Content: content{Parts: []part{}}, FinishReason: "STOP"},
		},
	}
	result := convertResponse(resp)
	if len(result.Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(result.Content))
	}
	if result.StopReason != "stop" {
		t.Errorf("stop reason = %q, want %q", result.StopReason, "stop")
	}
}

func TestConvertResponse_FinishReasonNoContent(t *testing.T) {
	// Candidate with finish reason but no actual content (e.g., safety filter).
	resp := &geminiResponse{
		Candidates: []candidate{
			{FinishReason: "SAFETY"},
		},
	}
	result := convertResponse(resp)
	if result.StopReason != "safety" {
		t.Errorf("stop reason = %q, want %q", result.StopReason, "safety")
	}
	if len(result.Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(result.Content))
	}
}

func TestConvertResponse_NilUsageMetadata(t *testing.T) {
	resp := &geminiResponse{
		Candidates: []candidate{
			{Content: content{Parts: []part{{Text: "Hi"}}}, FinishReason: "STOP"},
		},
	}
	result := convertResponse(resp)
	if result.Usage.InputTokens != 0 {
		t.Errorf("expected 0 InputTokens with nil usage, got %d", result.Usage.InputTokens)
	}
}

func TestConvertResponse_FunctionCallNilArgs(t *testing.T) {
	// FunctionCall with nil Args should not panic.
	resp := &geminiResponse{
		Candidates: []candidate{
			{Content: content{Parts: []part{
				{FunctionCall: &functionCall{Name: "noargs"}},
			}}},
		},
	}
	result := convertResponse(resp)
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Name != "noargs" {
		t.Errorf("name = %q, want %q", result.Content[0].Name, "noargs")
	}
	// nil Args should marshal to "null"
	if string(result.Content[0].Input) != "null" {
		t.Errorf("input = %q, want %q", string(result.Content[0].Input), "null")
	}
}

// ---------------------------------------------------------------------------
// parseSSEStream — error branch coverage
// ---------------------------------------------------------------------------

func TestParseSSEStream_MalformedJSON(t *testing.T) {
	// Malformed JSON data lines should be silently skipped.
	sseData := `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}

data: {invalid json}

data: {"candidates":[{"content":{"parts":[{"text":"Hello world"}]},"finishReason":"STOP"}]}

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

func TestParseSSEStream_NoCandidates(t *testing.T) {
	// Response with no candidates should be skipped (no delta).
	sseData := `data: {"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":0}}

data: {"candidates":[{"content":{"parts":[{"text":"Hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":1}}

`
	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(strings.NewReader(sseData), events)
	}()

	var text string
	var doneResp *types.CompletionResponse
	for ev := range events {
		if ev.Type == types.StreamEventDelta {
			text += ev.Content
		}
		if ev.Type == types.StreamEventDone {
			doneResp = ev.Response
		}
	}
	if text != "Hi" {
		t.Errorf("text = %q, want %q", text, "Hi")
	}
	if doneResp == nil {
		t.Fatal("expected done response")
	}
	if doneResp.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", doneResp.Usage.InputTokens)
	}
}

func TestParseSSEStream_EmptyStream(t *testing.T) {
	// Empty stream should emit start + done (empty response).
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
		t.Error("expected done event")
	}
}

func TestParseSSEStream_EmptyCandidateContent(t *testing.T) {
	// Candidate with empty content parts → no delta, no tool events.
	sseData := `data: {"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":0}}

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
	if deltaCount != 0 {
		t.Errorf("expected 0 deltas, got %d", deltaCount)
	}
}

func TestParseSSEStream_ReadError(t *testing.T) {
	// Read error mid-stream. Gemini parser doesn't emit error events
	// (unlike OpenAI) — it just emits done with whatever was accumulated.
	// Verify no panic.
	pr, pw := io.Pipe()

	go func() {
		pw.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}]}\n\n"))
		pw.CloseWithError(errors.New("connection reset"))
	}()

	events := make(chan types.StreamEvent, 20)
	go func() {
		defer close(events)
		parseSSEStream(pr, events)
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
	if text != "Hello" {
		t.Errorf("text = %q, want %q", text, "Hello")
	}
	if !gotDone {
		t.Error("expected done event")
	}
}

func TestParseSSEStream_NonDataLinesIgnored(t *testing.T) {
	sseData := `: comment line
event: message

data: {"candidates":[{"content":{"parts":[{"text":"Hi"}]},"finishReason":"STOP"}]}

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
