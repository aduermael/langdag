package langdag

import (
	"testing"

	"langdag.com/langdag/types"
)

func TestBuildResultEmitsToolUseFromDoneResponse(t *testing.T) {
	events := make(chan types.StreamEvent, 2)
	events <- types.StreamEvent{
		Type: types.StreamEventDone,
		Response: &types.CompletionResponse{
			Content: []types.ContentBlock{
				{Type: "tool_use", ID: "call_1", Name: "search", Input: []byte(`{"q":"test"}`)},
			},
			StopReason: "tool_calls",
		},
	}
	events <- types.StreamEvent{Type: types.StreamEventNodeSaved, NodeID: "node_1"}
	close(events)

	result := buildResult(events)
	var toolCalls []types.ContentBlock
	var done StreamChunk
	for chunk := range result.Stream {
		if chunk.ContentBlock != nil && chunk.ContentBlock.Type == "tool_use" {
			toolCalls = append(toolCalls, *chunk.ContentBlock)
		}
		if chunk.Done {
			done = chunk
		}
	}

	if len(toolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].ID != "call_1" || toolCalls[0].Name != "search" {
		t.Fatalf("tool call = %+v, want call_1/search", toolCalls[0])
	}
	if done.NodeID != "node_1" || done.StopReason != "tool_calls" {
		t.Fatalf("done = %+v, want node_1/tool_calls", done)
	}
}

func TestBuildResultDoesNotDuplicateStreamedToolUse(t *testing.T) {
	toolCall := types.ContentBlock{Type: "tool_use", ID: "call_1", Name: "search", Input: []byte(`{"q":"test"}`)}
	events := make(chan types.StreamEvent, 3)
	events <- types.StreamEvent{Type: types.StreamEventContentDone, ContentBlock: &toolCall}
	events <- types.StreamEvent{
		Type: types.StreamEventDone,
		Response: &types.CompletionResponse{
			Content:    []types.ContentBlock{toolCall},
			StopReason: "tool_calls",
		},
	}
	events <- types.StreamEvent{Type: types.StreamEventNodeSaved, NodeID: "node_1"}
	close(events)

	result := buildResult(events)
	var toolCalls int
	for chunk := range result.Stream {
		if chunk.ContentBlock != nil && chunk.ContentBlock.Type == "tool_use" {
			toolCalls++
		}
	}

	if toolCalls != 1 {
		t.Fatalf("tool calls = %d, want 1", toolCalls)
	}
}
