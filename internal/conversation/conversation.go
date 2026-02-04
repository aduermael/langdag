// Package conversation provides conversation management logic.
package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/langdag/langdag/internal/provider"
	"github.com/langdag/langdag/internal/storage"
	"github.com/langdag/langdag/pkg/types"
)

// Manager handles conversation operations (using unified DAG model).
type Manager struct {
	storage  storage.Storage
	provider provider.Provider
}

// NewManager creates a new conversation manager.
func NewManager(store storage.Storage, prov provider.Provider) *Manager {
	return &Manager{
		storage:  store,
		provider: prov,
	}
}

// CreateDAG creates a new DAG instance for a conversation.
func (m *Manager) CreateDAG(ctx context.Context, model, systemPrompt, title string) (*types.DAG, error) {
	now := time.Now()
	dag := &types.DAG{
		ID:           uuid.New().String(),
		Title:        title,
		Model:        model,
		SystemPrompt: systemPrompt,
		Status:       types.DAGStatusRunning,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := m.storage.CreateDAG(ctx, dag); err != nil {
		return nil, fmt.Errorf("failed to create DAG: %w", err)
	}

	return dag, nil
}

// GetDAG retrieves a DAG by ID.
func (m *Manager) GetDAG(ctx context.Context, id string) (*types.DAG, error) {
	return m.storage.GetDAG(ctx, id)
}

// ListDAGs returns all DAGs.
func (m *Manager) ListDAGs(ctx context.Context) ([]*types.DAG, error) {
	return m.storage.ListDAGs(ctx)
}

// DeleteDAG deletes a DAG.
func (m *Manager) DeleteDAG(ctx context.Context, id string) error {
	return m.storage.DeleteDAG(ctx, id)
}

// GetNodes retrieves all nodes in a DAG.
func (m *Manager) GetNodes(ctx context.Context, dagID string) ([]*types.DAGNode, error) {
	return m.storage.GetDAGNodes(ctx, dagID)
}

// AddUserMessage adds a user message to the DAG.
func (m *Manager) AddUserMessage(ctx context.Context, dagID, content string) (*types.DAGNode, error) {
	// Get the last node to determine parent and sequence
	lastNode, err := m.storage.GetLastDAGNode(ctx, dagID)
	if err != nil {
		return nil, fmt.Errorf("failed to get last node: %w", err)
	}

	sequence := 1
	var parentID string
	if lastNode != nil {
		sequence = lastNode.Sequence + 1
		parentID = lastNode.ID
	}

	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	node := &types.DAGNode{
		ID:        uuid.New().String(),
		DAGID:     dagID,
		ParentID:  parentID,
		Sequence:  sequence,
		NodeType:  types.NodeTypeUser,
		Content:   contentJSON,
		Status:    types.DAGStatusCompleted,
		CreatedAt: time.Now(),
	}

	if err := m.storage.AddDAGNode(ctx, node); err != nil {
		return nil, fmt.Errorf("failed to add user message: %w", err)
	}

	// Update DAG timestamp
	dag, err := m.storage.GetDAG(ctx, dagID)
	if err != nil {
		return nil, err
	}
	if dag != nil {
		m.storage.UpdateDAG(ctx, dag)
	}

	return node, nil
}

// SendMessage sends a message and gets a streaming response.
func (m *Manager) SendMessage(ctx context.Context, dagID, userMessage string) (<-chan types.StreamEvent, error) {
	// Get DAG
	dag, err := m.storage.GetDAG(ctx, dagID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DAG: %w", err)
	}
	if dag == nil {
		return nil, fmt.Errorf("DAG not found: %s", dagID)
	}

	// Add user message
	userNode, err := m.AddUserMessage(ctx, dagID, userMessage)
	if err != nil {
		return nil, err
	}

	// Build message history
	messages, err := m.buildMessageHistory(ctx, dagID)
	if err != nil {
		return nil, err
	}

	// Create completion request
	req := &types.CompletionRequest{
		Model:     dag.Model,
		Messages:  messages,
		System:    dag.SystemPrompt,
		MaxTokens: 4096,
		Tools:     dag.Tools,
	}

	// Stream response
	events, err := m.provider.Stream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to stream response: %w", err)
	}

	// Wrap the events channel to save the response when done
	outputEvents := make(chan types.StreamEvent, 100)
	go func() {
		defer close(outputEvents)

		var fullText string
		var response *types.CompletionResponse
		startTime := time.Now()

		for event := range events {
			outputEvents <- event

			switch event.Type {
			case types.StreamEventDelta:
				fullText += event.Content
			case types.StreamEventDone:
				response = event.Response
			}
		}

		// Save assistant response
		if response != nil || fullText != "" {
			m.saveAssistantResponse(ctx, dag, userNode, fullText, response, startTime)
		}
	}()

	return outputEvents, nil
}

// Complete sends a message and waits for the complete response.
func (m *Manager) Complete(ctx context.Context, dagID, userMessage string) (*types.CompletionResponse, error) {
	events, err := m.SendMessage(ctx, dagID, userMessage)
	if err != nil {
		return nil, err
	}

	var response *types.CompletionResponse
	for event := range events {
		if event.Type == types.StreamEventError {
			return nil, event.Error
		}
		if event.Type == types.StreamEventDone {
			response = event.Response
		}
	}

	return response, nil
}

// buildMessageHistory builds the message history for a DAG.
func (m *Manager) buildMessageHistory(ctx context.Context, dagID string) ([]types.Message, error) {
	nodes, err := m.storage.GetDAGNodes(ctx, dagID)
	if err != nil {
		return nil, err
	}

	messages := make([]types.Message, 0, len(nodes))
	for _, node := range nodes {
		var role string
		switch node.NodeType {
		case types.NodeTypeUser:
			role = "user"
		case types.NodeTypeAssistant:
			role = "assistant"
		case types.NodeTypeToolResult:
			role = "user" // Tool results are sent as user messages
		default:
			continue
		}

		messages = append(messages, types.Message{
			Role:    role,
			Content: node.Content,
		})
	}

	return messages, nil
}

// saveAssistantResponse saves the assistant's response as a DAG node.
func (m *Manager) saveAssistantResponse(ctx context.Context, dag *types.DAG, userNode *types.DAGNode, text string, response *types.CompletionResponse, startTime time.Time) {
	// Determine content to save
	var content json.RawMessage
	var tokensIn, tokensOut int

	// Extract token usage from response
	if response != nil {
		tokensIn = response.Usage.InputTokens
		tokensOut = response.Usage.OutputTokens
	}

	// Determine content format
	if response != nil && len(response.Content) > 0 {
		// Check if response contains tool calls
		hasToolUse := false
		for _, block := range response.Content {
			if block.Type == "tool_use" {
				hasToolUse = true
				break
			}
		}

		if hasToolUse {
			// Save full content blocks for tool use
			content, _ = json.Marshal(response.Content)
		} else {
			// Save just the text
			content, _ = json.Marshal(text)
		}
	} else {
		content, _ = json.Marshal(text)
	}

	latencyMs := int(time.Since(startTime).Milliseconds())

	node := &types.DAGNode{
		ID:        uuid.New().String(),
		DAGID:     dag.ID,
		ParentID:  userNode.ID,
		Sequence:  userNode.Sequence + 1,
		NodeType:  types.NodeTypeAssistant,
		Content:   content,
		Model:     dag.Model,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
		LatencyMs: latencyMs,
		Status:    types.DAGStatusCompleted,
		CreatedAt: time.Now(),
	}

	m.storage.AddDAGNode(ctx, node)
	m.storage.UpdateDAG(ctx, dag)
}

// UpdateTitle updates the DAG title.
func (m *Manager) UpdateTitle(ctx context.Context, dagID, title string) error {
	dag, err := m.storage.GetDAG(ctx, dagID)
	if err != nil {
		return err
	}
	if dag == nil {
		return fmt.Errorf("DAG not found: %s", dagID)
	}

	dag.Title = title
	return m.storage.UpdateDAG(ctx, dag)
}

// GenerateTitle generates a title for the DAG based on the first message.
func (m *Manager) GenerateTitle(text string) string {
	// Simple title generation: use first 50 chars of the first message
	if len(text) > 50 {
		return text[:47] + "..."
	}
	return text
}
