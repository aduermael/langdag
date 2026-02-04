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

// AddUserMessage adds a user message to the DAG after the last node.
func (m *Manager) AddUserMessage(ctx context.Context, dagID, content string) (*types.DAGNode, error) {
	// Get the last node to determine parent and sequence
	lastNode, err := m.storage.GetLastDAGNode(ctx, dagID)
	if err != nil {
		return nil, fmt.Errorf("failed to get last node: %w", err)
	}

	var parentID string
	if lastNode != nil {
		parentID = lastNode.ID
	}

	return m.AddUserMessageAfter(ctx, dagID, parentID, content)
}

// AddUserMessageAfter adds a user message after a specific node (for branching).
func (m *Manager) AddUserMessageAfter(ctx context.Context, dagID, parentNodeID, content string) (*types.DAGNode, error) {
	// Get max sequence to determine next sequence number
	nodes, err := m.storage.GetDAGNodes(ctx, dagID)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	maxSeq := 0
	for _, n := range nodes {
		if n.Sequence > maxSeq {
			maxSeq = n.Sequence
		}
	}

	contentJSON, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	node := &types.DAGNode{
		ID:        uuid.New().String(),
		DAGID:     dagID,
		ParentID:  parentNodeID,
		Sequence:  maxSeq + 1,
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

// SendMessage sends a message and gets a streaming response. Returns the assistant node ID.
func (m *Manager) SendMessage(ctx context.Context, dagID, userMessage string) (<-chan types.StreamEvent, error) {
	return m.SendMessageAfter(ctx, dagID, "", userMessage)
}

// SendMessageAfter sends a message after a specific node (for branching). Returns events channel.
// If parentNodeID is empty, appends to the last node.
func (m *Manager) SendMessageAfter(ctx context.Context, dagID, parentNodeID, userMessage string) (<-chan types.StreamEvent, error) {
	// Get DAG
	dag, err := m.storage.GetDAG(ctx, dagID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DAG: %w", err)
	}
	if dag == nil {
		return nil, fmt.Errorf("DAG not found: %s", dagID)
	}

	// Add user message
	var userNode *types.DAGNode
	if parentNodeID == "" {
		userNode, err = m.AddUserMessage(ctx, dagID, userMessage)
	} else {
		userNode, err = m.AddUserMessageAfter(ctx, dagID, parentNodeID, userMessage)
	}
	if err != nil {
		return nil, err
	}

	// Build message history from the user node (walking up the tree)
	messages, err := m.buildMessageHistoryFromNode(ctx, dagID, userNode.ID)
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

		// Save assistant response and send node_saved event
		if response != nil || fullText != "" {
			nodeID := m.saveAssistantResponse(ctx, dag, userNode, fullText, response, startTime)
			outputEvents <- types.StreamEvent{
				Type:   types.StreamEventNodeSaved,
				NodeID: nodeID,
			}
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

// buildMessageHistoryFromNode builds message history by walking up from a node to the root.
func (m *Manager) buildMessageHistoryFromNode(ctx context.Context, dagID, nodeID string) ([]types.Message, error) {
	// Get all nodes and build a lookup map
	nodes, err := m.storage.GetDAGNodes(ctx, dagID)
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[string]*types.DAGNode)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// Walk up from the given node to collect the path
	var path []*types.DAGNode
	currentID := nodeID
	for currentID != "" {
		node, ok := nodeMap[currentID]
		if !ok {
			break
		}
		path = append(path, node)
		currentID = node.ParentID
	}

	// Reverse to get chronological order (root to leaf)
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	// Convert to messages
	messages := make([]types.Message, 0, len(path))
	for _, node := range path {
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

// saveAssistantResponse saves the assistant's response as a DAG node and returns the node ID.
func (m *Manager) saveAssistantResponse(ctx context.Context, dag *types.DAG, userNode *types.DAGNode, text string, response *types.CompletionResponse, startTime time.Time) string {
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
	return node.ID
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

// GetNodeWithDAG finds a node by ID (or prefix) and returns both the node and its DAG.
func (m *Manager) GetNodeWithDAG(ctx context.Context, nodeID string) (*types.DAGNode, *types.DAG, error) {
	// Find the node by ID (try exact match first, then prefix)
	node, err := m.storage.GetDAGNode(ctx, nodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get node: %w", err)
	}
	if node == nil {
		// Try prefix match
		node, err = m.storage.GetDAGNodeByPrefix(ctx, nodeID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get node by prefix: %w", err)
		}
	}
	if node == nil {
		return nil, nil, fmt.Errorf("node not found: %s", nodeID)
	}

	// Get the DAG
	dag, err := m.storage.GetDAG(ctx, node.DAGID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get DAG: %w", err)
	}
	if dag == nil {
		return nil, nil, fmt.Errorf("DAG not found: %s", node.DAGID)
	}

	return node, dag, nil
}
