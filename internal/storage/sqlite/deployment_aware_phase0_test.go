package sqlite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"langdag.com/langdag/types"
)

func TestPhase0OldConversationSchemaMigratesProviderModelTokens(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "conversation", "testdata", "deployment_aware_phase0", "old_conversation_nodes_provider_model_tokens.json"))
	if err != nil {
		t.Fatalf("read old conversation fixture: %v", err)
	}

	var nodes []*types.Node
	if err := json.Unmarshal(data, &nodes); err != nil {
		t.Fatalf("unmarshal old conversation fixture: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "langdag-phase0-old-schema-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, err := store.db.ExecContext(ctx, migrations[i]); err != nil {
			store.Close()
			t.Fatalf("run migration %d: %v", i+1, err)
		}
	}

	for _, node := range nodes {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO nodes (
				id, parent_id, sequence, node_type, content,
				model, tokens_in, tokens_out, latency_ms, status,
				title, system_prompt, created_at,
				tokens_cache_read, tokens_cache_creation, tokens_reasoning,
				provider
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			node.ID, nullString(node.ParentID), node.Sequence, node.NodeType, node.Content,
			nullString(node.Model), node.TokensIn, node.TokensOut, node.LatencyMs, nullString(node.Status),
			nullString(node.Title), nullString(node.SystemPrompt), node.CreatedAt,
			node.TokensCacheRead, node.TokensCacheCreation, node.TokensReasoning,
			nullString(node.Provider)); err != nil {
			store.Close()
			t.Fatalf("insert old node %s: %v", node.ID, err)
		}
	}
	store.Close()

	migrated, err := New(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { migrated.Close() })
	if err := migrated.Init(ctx); err != nil {
		t.Fatalf("Init migrated old DB: %v", err)
	}

	assistant, err := migrated.GetNode(ctx, "phase0-assistant-1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if assistant.Provider != "anthropic" || assistant.Model != "claude-sonnet-4-6" {
		t.Fatalf("provider/model = %q/%q", assistant.Provider, assistant.Model)
	}
	if assistant.TokensIn != 1200 || assistant.TokensOut != 300 || assistant.TokensCacheRead != 200 || assistant.TokensCacheCreation != 100 || assistant.TokensReasoning != 40 {
		t.Fatalf("token fields not preserved after migration: %+v", assistant)
	}
	if len(assistant.Metadata) != 0 {
		t.Fatalf("old migrated node should not gain metadata: %s", string(assistant.Metadata))
	}
	if assistant.RootID != "phase0-root" {
		t.Fatalf("RootID = %q, want phase0-root", assistant.RootID)
	}

	ancestors, err := migrated.GetAncestors(ctx, "phase0-assistant-1")
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	if len(ancestors) != 2 || ancestors[0].ID != "phase0-root" || ancestors[1].ID != "phase0-assistant-1" {
		t.Fatalf("ancestors after migration = %+v", ancestors)
	}
}
