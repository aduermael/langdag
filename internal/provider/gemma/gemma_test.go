package gemma

import (
	"encoding/json"
	"strings"
	"testing"

	"langdag.com/langdag/types"
)

func TestProviderName(t *testing.T) {
	p := New("test-key")
	if got := p.Name(); got != "gemma" {
		t.Errorf("expected provider name 'gemma', got %q", got)
	}
}

func TestProviderModels(t *testing.T) {
	p := New("test-key")
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}

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

func TestBuildRequest_System(t *testing.T) {
	req := &types.CompletionRequest{
		System: "you are helpful",
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}
	body := buildRequest(req)
	if !strings.Contains(string(body), `"system_instruction"`) {
		t.Errorf("expected system_instruction in body, got: %s", string(body))
	}
}

func TestServerToolMapping(t *testing.T) {
	tools := []types.ToolDefinition{{Name: types.ServerToolWebSearch}}
	out := convertTools(tools)
	if len(out) != 1 {
		t.Fatalf("expected 1 tool entry, got %d", len(out))
	}
	if out[0].serverToolName != "google_search" {
		t.Errorf("expected web_search to map to google_search, got %q", out[0].serverToolName)
	}
}
