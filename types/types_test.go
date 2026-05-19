package types

import (
	"encoding/json"
	"testing"
)

func TestServerToolWebSearchConstant(t *testing.T) {
	if ServerToolWebSearch != "web_search" {
		t.Errorf("ServerToolWebSearch = %q, want %q", ServerToolWebSearch, "web_search")
	}
}

func TestIsClientTool(t *testing.T) {
	tests := []struct {
		name string
		tool ToolDefinition
		want bool
	}{
		{
			name: "with InputSchema is client tool",
			tool: ToolDefinition{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
			},
			want: true,
		},
		{
			name: "without InputSchema is server tool",
			tool: ToolDefinition{Name: "web_search"},
			want: false,
		},
		{
			name: "nil InputSchema is server tool",
			tool: ToolDefinition{Name: "code_execution", InputSchema: nil},
			want: false,
		},
		{
			name: "empty InputSchema is server tool",
			tool: ToolDefinition{Name: "web_search", InputSchema: json.RawMessage{}},
			want: false,
		},
		{
			name: "null InputSchema is server tool",
			tool: ToolDefinition{Name: "web_search", InputSchema: json.RawMessage(`null`)},
			want: false,
		},
		{
			name: "web_search with schema overrides as client tool",
			tool: ToolDefinition{
				Name:        ServerToolWebSearch,
				Description: "My custom search",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tool.IsClientTool(); got != tt.want {
				t.Errorf("IsClientTool() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ContentBlock.ToolResultContent()
// ---------------------------------------------------------------------------

func TestContentBlock_ToolResultContent_PlainString(t *testing.T) {
	b := ContentBlock{
		Type:      "tool_result",
		ToolUseID: "toolu_001",
		Content:   "hello",
	}
	got := b.ToolResultContent()
	if got == nil {
		t.Fatal("ToolResultContent() returned nil, want JSON string")
	}
	// Should be a JSON-encoded string: "hello"
	if string(got) != `"hello"` {
		t.Errorf("ToolResultContent() = %s, want %s", string(got), `"hello"`)
	}
}

func TestContentBlock_ToolResultContent_JSON(t *testing.T) {
	structured := json.RawMessage(`{"result":"sunny","temp_c":22,"metadata":{"source":"weather_api"}}`)
	b := ContentBlock{
		Type:        "tool_result",
		ToolUseID:   "toolu_002",
		ContentJSON: structured,
	}
	got := b.ToolResultContent()
	if got == nil {
		t.Fatal("ToolResultContent() returned nil, want structured JSON")
	}
	if string(got) != string(structured) {
		t.Errorf("ToolResultContent() = %s, want %s", string(got), string(structured))
	}
}

func TestContentBlock_ToolResultContent_JSONOverridesString(t *testing.T) {
	structured := json.RawMessage(`{"tokens":150,"conversation_id":"conv_abc"}`)
	b := ContentBlock{
		Type:        "tool_result",
		ToolUseID:   "toolu_003",
		Content:     "this should be ignored",
		ContentJSON: structured,
	}
	got := b.ToolResultContent()
	if got == nil {
		t.Fatal("ToolResultContent() returned nil")
	}
	// ContentJSON must take priority over Content
	if string(got) != string(structured) {
		t.Errorf("ToolResultContent() = %s, want %s (ContentJSON should override Content)", string(got), string(structured))
	}
}

func TestContentBlock_ToolResultContent_BothEmpty(t *testing.T) {
	b := ContentBlock{
		Type:      "tool_result",
		ToolUseID: "toolu_004",
	}
	got := b.ToolResultContent()
	if got != nil {
		t.Errorf("ToolResultContent() = %s, want nil when both Content and ContentJSON are empty", string(got))
	}
}

func TestContentBlock_ToolResultContent_MarshalRoundTrip(t *testing.T) {
	// Verify that a ContentBlock with ContentJSON survives JSON marshal/unmarshal.
	original := ContentBlock{
		Type:        "tool_result",
		ToolUseID:   "toolu_005",
		Content:     "fallback text",
		ContentJSON: json.RawMessage(`{"status":"ok","data":[1,2,3]}`),
		IsError:     false,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded ContentBlock
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify all fields survived
	if decoded.Type != original.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, original.Type)
	}
	if decoded.ToolUseID != original.ToolUseID {
		t.Errorf("ToolUseID = %q, want %q", decoded.ToolUseID, original.ToolUseID)
	}
	if decoded.Content != original.Content {
		t.Errorf("Content = %q, want %q", decoded.Content, original.Content)
	}
	if string(decoded.ContentJSON) != string(original.ContentJSON) {
		t.Errorf("ContentJSON = %s, want %s", string(decoded.ContentJSON), string(original.ContentJSON))
	}

	// ToolResultContent should still return ContentJSON
	got := decoded.ToolResultContent()
	if string(got) != string(original.ContentJSON) {
		t.Errorf("after round-trip, ToolResultContent() = %s, want %s", string(got), string(original.ContentJSON))
	}
}

func TestContentBlock_ToolResultContent_StringWithSpecialChars(t *testing.T) {
	// Verify that Content with quotes and special characters is correctly
	// JSON-encoded by ToolResultContent.
	b := ContentBlock{
		Type:      "tool_result",
		ToolUseID: "toolu_006",
		Content:   `result: "value" with\nnewline`,
	}
	got := b.ToolResultContent()
	if got == nil {
		t.Fatal("ToolResultContent() returned nil")
	}
	// Unmarshal back to string and check it matches
	var s string
	if err := json.Unmarshal(got, &s); err != nil {
		t.Fatalf("failed to unmarshal ToolResultContent as string: %v", err)
	}
	if s != b.Content {
		t.Errorf("round-trip string = %q, want %q", s, b.Content)
	}
}
