package cli

import (
	"testing"

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
