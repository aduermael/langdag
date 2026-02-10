// Package executor provides workflow execution functionality.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/langdag/langdag/internal/provider"
	"github.com/langdag/langdag/internal/storage"
	"github.com/langdag/langdag/internal/workflow"
	"github.com/langdag/langdag/pkg/types"
)

// Executor handles workflow execution.
type Executor struct {
	storage  storage.Storage
	provider provider.Provider
}

// NewExecutor creates a new executor.
func NewExecutor(store storage.Storage, prov provider.Provider) *Executor {
	return &Executor{
		storage:  store,
		provider: prov,
	}
}

// ExecuteOptions contains options for workflow execution.
type ExecuteOptions struct {
	Stream bool
}

// ExecutionEvent represents an event during workflow execution.
type ExecutionEvent struct {
	Type       string          `json:"type"` // "node_start", "node_complete", "stream", "error", "done"
	NodeID     string          `json:"node_id,omitempty"`
	NodeType   types.NodeType  `json:"node_type,omitempty"`
	Content    string          `json:"content,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	RootNodeID string          `json:"root_node_id,omitempty"`
}

// Execute executes a workflow with the given input, creating nodes in the tree.
func (e *Executor) Execute(ctx context.Context, wf *types.Workflow, input json.RawMessage, opts ExecuteOptions) (<-chan ExecutionEvent, error) {
	result := workflow.Validate(wf)
	if !result.Valid {
		return nil, fmt.Errorf("invalid workflow: %s", result.FormatErrors())
	}

	order, err := workflow.TopologicalSort(wf)
	if err != nil {
		return nil, err
	}

	// Create a root node for this workflow execution
	rootNode := &types.Node{
		ID:        uuid.New().String(),
		Sequence:  0,
		NodeType:  types.NodeTypeSystem,
		Content:   string(input),
		Model:     wf.Defaults.Model,
		Status:    "running",
		Title:     wf.Name,
		CreatedAt: time.Now(),
	}
	if err := e.storage.CreateNode(ctx, rootNode); err != nil {
		return nil, fmt.Errorf("failed to create root node: %w", err)
	}

	events := make(chan ExecutionEvent, 100)

	go func() {
		defer close(events)
		e.executeWorkflow(ctx, wf, rootNode, order, input, opts, events)
	}()

	return events, nil
}

// executeWorkflow executes the workflow nodes in order.
func (e *Executor) executeWorkflow(ctx context.Context, wf *types.Workflow, rootNode *types.Node, order []string, input json.RawMessage, opts ExecuteOptions, events chan<- ExecutionEvent) {
	state := make(map[string]json.RawMessage)
	state["input"] = input

	for seq, nodeID := range order {
		node := workflow.GetNode(wf, nodeID)
		if node == nil {
			continue
		}

		events <- ExecutionEvent{
			Type:     "node_start",
			NodeID:   nodeID,
			NodeType: node.Type,
		}

		output, err := e.executeNode(ctx, wf, rootNode, node, state, opts, events, seq)
		if err != nil {
			rootNode.Status = "failed"
			e.storage.UpdateNode(ctx, rootNode)
			events <- ExecutionEvent{
				Type:       "error",
				NodeID:     nodeID,
				Error:      err.Error(),
				RootNodeID: rootNode.ID,
			}
			return
		}

		if output != nil {
			state[nodeID] = output
		}

		events <- ExecutionEvent{
			Type:     "node_complete",
			NodeID:   nodeID,
			NodeType: node.Type,
			Output:   output,
		}
	}

	// Find output node and get final result
	var finalOutput json.RawMessage
	outputNode := workflow.GetOutputNode(wf)
	if outputNode != nil {
		parents := workflow.GetParentNodes(wf, outputNode.ID)
		if len(parents) > 0 {
			finalOutput = state[parents[0].ID]
		}
	}

	rootNode.Status = "completed"
	rootNode.Content = string(finalOutput)
	e.storage.UpdateNode(ctx, rootNode)

	events <- ExecutionEvent{
		Type:       "done",
		RootNodeID: rootNode.ID,
		Output:     finalOutput,
	}
}

// executeNode executes a single workflow node.
func (e *Executor) executeNode(ctx context.Context, wf *types.Workflow, rootNode *types.Node, node *types.WorkflowNode, state map[string]json.RawMessage, opts ExecuteOptions, events chan<- ExecutionEvent, seq int) (json.RawMessage, error) {
	switch node.Type {
	case types.NodeTypeInput:
		return state["input"], nil

	case types.NodeTypeOutput:
		parents := workflow.GetParentNodes(wf, node.ID)
		if len(parents) > 0 {
			return state[parents[0].ID], nil
		}
		return nil, nil

	case types.NodeTypeLLM:
		return e.executeLLMNode(ctx, wf, rootNode, node, state, opts, events, seq)

	case types.NodeTypeMerge:
		return e.executeMergeNode(wf, node, state)

	case types.NodeTypeBranch:
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported node type: %s", node.Type)
	}
}

// executeLLMNode executes an LLM node.
func (e *Executor) executeLLMNode(ctx context.Context, wf *types.Workflow, rootNode *types.Node, node *types.WorkflowNode, state map[string]json.RawMessage, opts ExecuteOptions, events chan<- ExecutionEvent, seq int) (json.RawMessage, error) {
	prompt := e.substituteTemplate(node.Prompt, state)

	model := node.Model
	if model == "" {
		model = wf.Defaults.Model
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	var tools []types.ToolDefinition
	for _, toolName := range node.Tools {
		for _, tool := range wf.Tools {
			if tool.Name == toolName {
				tools = append(tools, tool)
				break
			}
		}
	}

	req := &types.CompletionRequest{
		Model: model,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(fmt.Sprintf("%q", prompt))},
		},
		System:    node.System,
		MaxTokens: wf.Defaults.MaxTokens,
		Tools:     tools,
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	startTime := time.Now()
	var output json.RawMessage
	var tokensIn, tokensOut int
	var err error

	if opts.Stream {
		output, tokensIn, tokensOut, err = e.streamLLMNode(ctx, req, node.ID, events)
	} else {
		resp, respErr := e.provider.Complete(ctx, req)
		if respErr != nil {
			return nil, respErr
		}

		var text string
		for _, block := range resp.Content {
			if block.Type == "text" {
				text = block.Text
				break
			}
		}
		output = json.RawMessage(fmt.Sprintf("%q", text))
		tokensIn = resp.Usage.InputTokens
		tokensOut = resp.Usage.OutputTokens
	}

	if err != nil {
		return nil, err
	}

	// Save as child node of the root
	childNode := &types.Node{
		ID:        uuid.New().String(),
		ParentID:  rootNode.ID,
		Sequence:  seq + 1,
		NodeType:  types.NodeTypeAssistant,
		Content:   prompt,
		Model:     model,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
		Status:    "completed",
		LatencyMs: int(time.Since(startTime).Milliseconds()),
		CreatedAt: time.Now(),
	}
	e.storage.CreateNode(ctx, childNode)

	return output, nil
}

// streamLLMNode streams an LLM node execution.
func (e *Executor) streamLLMNode(ctx context.Context, req *types.CompletionRequest, nodeID string, events chan<- ExecutionEvent) (json.RawMessage, int, int, error) {
	stream, err := e.provider.Stream(ctx, req)
	if err != nil {
		return nil, 0, 0, err
	}

	var fullText string
	var tokensIn, tokensOut int
	for event := range stream {
		switch event.Type {
		case types.StreamEventDelta:
			fullText += event.Content
			events <- ExecutionEvent{
				Type:    "stream",
				NodeID:  nodeID,
				Content: event.Content,
			}
		case types.StreamEventDone:
			if event.Response != nil {
				tokensIn = event.Response.Usage.InputTokens
				tokensOut = event.Response.Usage.OutputTokens
			}
		case types.StreamEventError:
			return nil, 0, 0, event.Error
		}
	}

	return json.RawMessage(fmt.Sprintf("%q", fullText)), tokensIn, tokensOut, nil
}

// executeMergeNode executes a merge node.
func (e *Executor) executeMergeNode(wf *types.Workflow, node *types.WorkflowNode, state map[string]json.RawMessage) (json.RawMessage, error) {
	parents := workflow.GetParentNodes(wf, node.ID)

	merged := make(map[string]json.RawMessage)
	for _, parent := range parents {
		if output, ok := state[parent.ID]; ok {
			merged[parent.ID] = output
		}
	}

	return json.Marshal(merged)
}

// substituteTemplate substitutes template variables in a string.
func (e *Executor) substituteTemplate(template string, state map[string]json.RawMessage) string {
	result := template

	for key, value := range state {
		var strVal string
		if err := json.Unmarshal(value, &strVal); err == nil {
			result = strings.ReplaceAll(result, "{{"+key+"}}", strVal)
			result = strings.ReplaceAll(result, "{{"+key+".output}}", strVal)
		} else {
			result = strings.ReplaceAll(result, "{{"+key+"}}", string(value))
			result = strings.ReplaceAll(result, "{{"+key+".output}}", string(value))
		}
	}

	return result
}
