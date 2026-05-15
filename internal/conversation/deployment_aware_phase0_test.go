package conversation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"langdag.com/langdag/internal/provider/mock"
	"langdag.com/langdag/internal/storage/sqlite"
	"langdag.com/langdag/types"
)

func TestPhase0OldConversationNodeFixturePreservesProviderModelTokens(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "deployment_aware_phase0", "old_conversation_nodes_provider_model_tokens.json"))
	if err != nil {
		t.Fatalf("read old conversation fixture: %v", err)
	}

	var nodes []*types.Node
	if err := json.Unmarshal(data, &nodes); err != nil {
		t.Fatalf("unmarshal old conversation fixture: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("fixture nodes = %d, want 2", len(nodes))
	}

	assistant := nodes[1]
	if assistant.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", assistant.Provider)
	}
	if assistant.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want claude-sonnet-4-6", assistant.Model)
	}
	if assistant.TokensIn != 1200 || assistant.TokensOut != 300 || assistant.TokensCacheRead != 200 || assistant.TokensCacheCreation != 100 || assistant.TokensReasoning != 40 {
		t.Errorf("token fields not preserved: %+v", assistant)
	}
	if len(assistant.Metadata) != 0 {
		t.Errorf("old assistant fixture should not have deployment metadata yet: %s", string(assistant.Metadata))
	}

	messages := buildMessages(nodes)
	if len(messages) != 2 {
		t.Fatalf("buildMessages returned %d messages, want 2", len(messages))
	}
	if messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("roles = %q/%q, want user/assistant", messages[0].Role, messages[1].Role)
	}

	tmpFile, err := os.CreateTemp("", "langdag-phase0-current-schema-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := sqlite.New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	for _, node := range nodes {
		if err := store.CreateNode(ctx, node); err != nil {
			t.Fatalf("CreateNode(%s): %v", node.ID, err)
		}
	}

	manager := NewManager(store, mock.New(mock.Config{}))
	stored, err := manager.ResolveNode(ctx, "phase0-assistant-1")
	if err != nil {
		t.Fatalf("ResolveNode: %v", err)
	}
	if stored.Provider != "anthropic" || stored.Model != "claude-sonnet-4-6" {
		t.Fatalf("stored provider/model = %q/%q", stored.Provider, stored.Model)
	}
	if stored.TokensIn != 1200 || stored.TokensOut != 300 || stored.TokensCacheRead != 200 || stored.TokensCacheCreation != 100 || stored.TokensReasoning != 40 {
		t.Fatalf("stored token fields not preserved: %+v", stored)
	}
	if len(stored.Metadata) != 0 {
		t.Fatalf("stored old node should not gain deployment metadata: %s", string(stored.Metadata))
	}
}
