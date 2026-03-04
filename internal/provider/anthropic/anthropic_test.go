package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/langdag/langdag/pkg/types"
)

func TestDirectProviderName(t *testing.T) {
	p := New("test-key")
	if p.Name() != "anthropic" {
		t.Errorf("expected name 'anthropic', got '%s'", p.Name())
	}
}

func TestDirectProviderModels(t *testing.T) {
	p := New("test-key")
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	// Verify first model has required fields
	m := models[0]
	if m.ID == "" || m.Name == "" || m.ContextWindow == 0 {
		t.Errorf("model missing required fields: %+v", m)
	}
}

func TestVertexProviderName(t *testing.T) {
	// VertexProvider can't be constructed without GCP credentials,
	// so test the struct directly
	p := &VertexProvider{}
	if p.Name() != "anthropic-vertex" {
		t.Errorf("expected name 'anthropic-vertex', got '%s'", p.Name())
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

func TestBedrockProviderName(t *testing.T) {
	p := &BedrockProvider{}
	if p.Name() != "anthropic-bedrock" {
		t.Errorf("expected name 'anthropic-bedrock', got '%s'", p.Name())
	}
}

func TestBedrockProviderModels(t *testing.T) {
	p := &BedrockProvider{}
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

func TestBuildParams(t *testing.T) {
	req := &types.CompletionRequest{
		Model:       "claude-sonnet-4-20250514",
		Messages:    []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		System:      "You are helpful.",
		MaxTokens:   1024,
		Temperature: 0.7,
		StopSeqs:    []string{"END"},
	}

	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams failed: %v", err)
	}

	if string(params.Model) != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got '%s'", params.Model)
	}
	if params.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %d", params.MaxTokens)
	}
	if len(params.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(params.Messages))
	}
	if len(params.System) != 1 {
		t.Errorf("expected 1 system block, got %d", len(params.System))
	}
	if len(params.StopSequences) != 1 {
		t.Errorf("expected 1 stop sequence, got %d", len(params.StopSequences))
	}
}

func TestBuildParamsWithTools(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
	req := &types.CompletionRequest{
		Model:     "claude-sonnet-4-20250514",
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
		MaxTokens: 1024,
		Tools: []types.ToolDefinition{
			{Name: "search", Description: "Search the web", InputSchema: schema},
		},
	}

	params, err := buildParams(req)
	if err != nil {
		t.Fatalf("buildParams failed: %v", err)
	}

	if len(params.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(params.Tools))
	}
}

func TestConvertMessagesString(t *testing.T) {
	msgs := []types.Message{
		{Role: "user", Content: json.RawMessage(`"Hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"Hi there"`)},
	}

	result, err := convertMessages(msgs)
	if err != nil {
		t.Fatalf("convertMessages failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
	}
}

func TestConvertMessagesContentBlocks(t *testing.T) {
	blocks := `[{"type":"text","text":"hello"},{"type":"image","media_type":"image/png","data":"base64data"}]`
	msgs := []types.Message{
		{Role: "user", Content: json.RawMessage(blocks)},
	}

	result, err := convertMessages(msgs)
	if err != nil {
		t.Fatalf("convertMessages failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}
