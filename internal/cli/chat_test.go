package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"langdag.com/langdag"
	"langdag.com/langdag/internal/config"
)

func TestConvertRoutingStagesPreservesExplicitEmptyDefault(t *testing.T) {
	stages := convertRoutingStages([]config.RoutingStage{})
	if stages == nil || len(stages) != 0 {
		t.Fatalf("empty route was not preserved: %+v", stages)
	}

	stages = convertRoutingStages(nil)
	if stages != nil {
		t.Fatalf("nil route should remain nil: %+v", stages)
	}

	stageMap := convertRoutingStageMap(map[string][]config.RoutingStage{"openai": {}})
	stages, ok := stageMap["openai"]
	if !ok || stages == nil || len(stages) != 0 {
		t.Fatalf("empty provider route was not preserved: %+v", stageMap)
	}
}

func TestNewLibraryClientUsesEmbeddedRuntimeCatalog(t *testing.T) {
	const canonicalID = "openai/gpt-4.1-2025-04-14"
	const nativeID = "gpt-4.1-2025-04-14"

	t.Setenv("LANGDAG_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("LANGDAG_STORAGE_PATH", filepath.Join(t.TempDir(), "cli.db"))

	var requestedModel string
	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestedModel = req.Model
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, `data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"cache cli"}

data: {"type":"response.completed","response":{"id":"resp-cache-cli","model":%q,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"cache cli"}]}],"usage":{"input_tokens":5,"output_tokens":2},"status":"completed"}}
`, nativeID)
	}))
	defer openAI.Close()
	t.Setenv("OPENAI_BASE_URL", openAI.URL)

	client, err := newLibraryClient(context.Background())
	if err != nil {
		t.Fatalf("newLibraryClient: %v", err)
	}
	defer client.Close()

	result, err := client.Prompt(context.Background(), "use cli runtime catalog", langdag.WithModel(canonicalID))
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	for chunk := range result.Stream {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
	}
	if requestedModel != nativeID {
		t.Fatalf("request model = %q, want embedded catalog native id %q", requestedModel, nativeID)
	}
}
