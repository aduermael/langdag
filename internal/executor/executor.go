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
	Type     string          `json:"type"`     // "node_start", "node_complete", "stream", "error", "done"
	NodeID   string          `json:"node_id,omitempty"`
	NodeType types.NodeType  `json:"node_type,omitempty"`
	Content  string          `json:"content,omitempty"`
	Output   json.RawMessage `json:"output,omitempty"`
	Error    string          `json:"error,omitempty"`
	DAG      *types.DAG      `json:"dag,omitempty"`
}

// Execute executes a workflow with the given input, creating a new DAG instance.
func (e *Executor) Execute(ctx context.Context, wf *types.Workflow, input json.RawMessage, opts ExecuteOptions) (<-chan ExecutionEvent, error) {
	// Validate the workflow
	result := workflow.Validate(wf)
	if !result.Valid {
		return nil, fmt.Errorf("invalid workflow: %s", result.FormatErrors())
	}

	// Get topological order
	order, err := workflow.TopologicalSort(wf)
	if err != nil {
		return nil, err
	}

	// Create DAG instance
	now := time.Now()
	dag := &types.DAG{
		ID:         uuid.New().String(),
		Title:      wf.Name,
		WorkflowID: wf.ID,
		Status:     types.DAGStatusRunning,
		Input:      input,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := e.storage.CreateDAG(ctx, dag); err != nil {
		return nil, fmt.Errorf("failed to create DAG: %w", err)
	}

	events := make(chan ExecutionEvent, 100)

	go func() {
		defer close(events)
		e.executeWorkflow(ctx, wf, dag, order, input, opts, events)
	}()

	return events, nil
}

// executeWorkflow executes the workflow nodes in order.
func (e *Executor) executeWorkflow(ctx context.Context, wf *types.Workflow, dag *types.DAG, order []string, input json.RawMessage, opts ExecuteOptions, events chan<- ExecutionEvent) {
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

		output, err := e.executeNode(ctx, wf, dag, node, state, opts, events, seq)
		if err != nil {
			e.handleError(ctx, dag, nodeID, err, events)
			return
		}

		// Store output in state
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
		// Get the parent of output node
		parents := workflow.GetParentNodes(wf, outputNode.ID)
		if len(parents) > 0 {
			finalOutput = state[parents[0].ID]
		}
	}

	// Update DAG
	now := time.Now()
	dag.Status = types.DAGStatusCompleted
	dag.UpdatedAt = now
	dag.Output = finalOutput

	e.storage.UpdateDAG(ctx, dag)

	events <- ExecutionEvent{
		Type:   "done",
		DAG:    dag,
		Output: finalOutput,
	}
}

// executeNode executes a single node.
func (e *Executor) executeNode(ctx context.Context, wf *types.Workflow, dag *types.DAG, node *types.Node, state map[string]json.RawMessage, opts ExecuteOptions, events chan<- ExecutionEvent, seq int) (json.RawMessage, error) {
	startTime := time.Now()

	switch node.Type {
	case types.NodeTypeInput:
		return state["input"], nil

	case types.NodeTypeOutput:
		// Output node just passes through
		parents := workflow.GetParentNodes(wf, node.ID)
		if len(parents) > 0 {
			return state[parents[0].ID], nil
		}
		return nil, nil

	case types.NodeTypeLLM:
		return e.executeLLMNode(ctx, wf, dag, node, state, opts, events, startTime, seq)

	case types.NodeTypeMerge:
		return e.executeMergeNode(wf, node, state)

	case types.NodeTypeBranch:
		// Branch nodes are handled by the scheduler
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported node type: %s", node.Type)
	}
}

// executeLLMNode executes an LLM node.
func (e *Executor) executeLLMNode(ctx context.Context, wf *types.Workflow, dag *types.DAG, node *types.Node, state map[string]json.RawMessage, opts ExecuteOptions, events chan<- ExecutionEvent, startTime time.Time, seq int) (json.RawMessage, error) {
	// Build prompt with template substitution
	prompt := e.substituteTemplate(node.Prompt, state)

	// Determine model
	model := node.Model
	if model == "" {
		model = wf.Defaults.Model
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	// Build tools
	var tools []types.ToolDefinition
	for _, toolName := range node.Tools {
		for _, tool := range wf.Tools {
			if tool.Name == toolName {
				tools = append(tools, tool)
				break
			}
		}
	}

	// Create completion request
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

	var output json.RawMessage
	var err error

	if opts.Stream {
		output, err = e.streamLLMNode(ctx, req, node.ID, events)
	} else {
		resp, err := e.provider.Complete(ctx, req)
		if err != nil {
			return nil, err
		}

		// Extract text content
		var text string
		for _, block := range resp.Content {
			if block.Type == "text" {
				text = block.Text
				break
			}
		}
		output = json.RawMessage(fmt.Sprintf("%q", text))
	}

	if err != nil {
		return nil, err
	}

	// Save node execution as DAG node
	dagNode := &types.DAGNode{
		ID:        uuid.New().String(),
		DAGID:     dag.ID,
		Sequence:  seq,
		NodeType:  node.Type,
		Content:   json.RawMessage(fmt.Sprintf("%q", prompt)),
		Model:     model,
		Status:    types.DAGStatusCompleted,
		Input:     state["input"],
		Output:    output,
		LatencyMs: int(time.Since(startTime).Milliseconds()),
		CreatedAt: time.Now(),
	}
	e.storage.AddDAGNode(ctx, dagNode)

	return output, nil
}

// streamLLMNode streams an LLM node execution.
func (e *Executor) streamLLMNode(ctx context.Context, req *types.CompletionRequest, nodeID string, events chan<- ExecutionEvent) (json.RawMessage, error) {
	stream, err := e.provider.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	var fullText string
	for event := range stream {
		switch event.Type {
		case types.StreamEventDelta:
			fullText += event.Content
			events <- ExecutionEvent{
				Type:    "stream",
				NodeID:  nodeID,
				Content: event.Content,
			}
		case types.StreamEventError:
			return nil, event.Error
		}
	}

	return json.RawMessage(fmt.Sprintf("%q", fullText)), nil
}

// executeMergeNode executes a merge node.
func (e *Executor) executeMergeNode(wf *types.Workflow, node *types.Node, state map[string]json.RawMessage) (json.RawMessage, error) {
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
		// Try to unmarshal as string first
		var strVal string
		if err := json.Unmarshal(value, &strVal); err == nil {
			result = strings.ReplaceAll(result, "{{"+key+"}}", strVal)
			result = strings.ReplaceAll(result, "{{"+key+".output}}", strVal)
		} else {
			// Use raw JSON
			result = strings.ReplaceAll(result, "{{"+key+"}}", string(value))
			result = strings.ReplaceAll(result, "{{"+key+".output}}", string(value))
		}
	}

	return result
}

// handleError handles an execution error.
func (e *Executor) handleError(ctx context.Context, dag *types.DAG, nodeID string, err error, events chan<- ExecutionEvent) {
	now := time.Now()
	dag.Status = types.DAGStatusFailed
	dag.UpdatedAt = now

	e.storage.UpdateDAG(ctx, dag)

	events <- ExecutionEvent{
		Type:   "error",
		NodeID: nodeID,
		Error:  err.Error(),
		DAG:    dag,
	}
}

// GetDAG retrieves a DAG by ID.
func (e *Executor) GetDAG(ctx context.Context, id string) (*types.DAG, error) {
	return e.storage.GetDAG(ctx, id)
}

// ListDAGs lists all DAGs.
func (e *Executor) ListDAGs(ctx context.Context) ([]*types.DAG, error) {
	return e.storage.ListDAGs(ctx)
}
