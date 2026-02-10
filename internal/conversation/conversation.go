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

// Manager handles conversation operations using the unified node model.
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

// Prompt creates a new conversation tree with the given message.
// It creates a root user node, sends to the LLM, and streams the response.
// The assistant node is saved when the stream completes.
func (m *Manager) Prompt(ctx context.Context, message, model, systemPrompt string) (<-chan types.StreamEvent, error) {
	rootNode := &types.Node{
		ID:           uuid.New().String(),
		Sequence:     0,
		NodeType:     types.NodeTypeUser,
		Content:      message,
		Model:        model,
		Status:       "completed",
		Title:        GenerateTitle(message),
		SystemPrompt: systemPrompt,
		CreatedAt:    time.Now(),
	}
	if err := m.storage.CreateNode(ctx, rootNode); err != nil {
		return nil, fmt.Errorf("failed to create root node: %w", err)
	}

	messages := []types.Message{
		{Role: "user", Content: json.RawMessage(fmt.Sprintf("%q", message))},
	}

	return m.streamResponse(ctx, rootNode, messages, model, systemPrompt)
}

// PromptFrom continues a conversation from an existing node.
// It creates a user child node, builds message history by walking to the root,
// sends to the LLM, and streams the response.
func (m *Manager) PromptFrom(ctx context.Context, parentNodeID, message, model string) (<-chan types.StreamEvent, error) {
	// Get ancestors (path from root to parentNode)
	ancestors, err := m.storage.GetAncestors(ctx, parentNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ancestors: %w", err)
	}
	if len(ancestors) == 0 {
		return nil, fmt.Errorf("node not found: %s", parentNodeID)
	}

	root := ancestors[0]
	lastNode := ancestors[len(ancestors)-1]

	// Determine model (request override > root default)
	if model == "" {
		model = root.Model
	}

	// Create user node as child of parentNode
	userNode := &types.Node{
		ID:        uuid.New().String(),
		ParentID:  parentNodeID,
		Sequence:  lastNode.Sequence + 1,
		NodeType:  types.NodeTypeUser,
		Content:   message,
		Status:    "completed",
		CreatedAt: time.Now(),
	}
	if err := m.storage.CreateNode(ctx, userNode); err != nil {
		return nil, fmt.Errorf("failed to create user node: %w", err)
	}

	// Build message history from ancestors + this new message
	messages := buildMessages(ancestors)
	messages = append(messages, types.Message{
		Role:    "user",
		Content: json.RawMessage(fmt.Sprintf("%q", message)),
	})

	return m.streamResponse(ctx, userNode, messages, model, root.SystemPrompt)
}

// streamResponse sends messages to the LLM and wraps the provider events,
// saving the assistant node when the stream completes.
func (m *Manager) streamResponse(ctx context.Context, parentNode *types.Node, messages []types.Message, model, systemPrompt string) (<-chan types.StreamEvent, error) {
	req := &types.CompletionRequest{
		Model:     model,
		Messages:  messages,
		System:    systemPrompt,
		MaxTokens: 4096,
	}

	providerEvents, err := m.provider.Stream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to stream response: %w", err)
	}

	events := make(chan types.StreamEvent, 100)
	go func() {
		defer close(events)

		var fullText string
		var response *types.CompletionResponse
		startTime := time.Now()

		for event := range providerEvents {
			events <- event
			switch event.Type {
			case types.StreamEventDelta:
				fullText += event.Content
			case types.StreamEventDone:
				response = event.Response
			}
		}

		if response != nil || fullText != "" {
			assistantNode := &types.Node{
				ID:        uuid.New().String(),
				ParentID:  parentNode.ID,
				Sequence:  parentNode.Sequence + 1,
				NodeType:  types.NodeTypeAssistant,
				Content:   fullText,
				Model:     model,
				Status:    "completed",
				LatencyMs: int(time.Since(startTime).Milliseconds()),
				CreatedAt: time.Now(),
			}
			if response != nil {
				assistantNode.TokensIn = response.Usage.InputTokens
				assistantNode.TokensOut = response.Usage.OutputTokens
			}
			m.storage.CreateNode(ctx, assistantNode)
			events <- types.StreamEvent{
				Type:   types.StreamEventNodeSaved,
				NodeID: assistantNode.ID,
			}
		}
	}()

	return events, nil
}

// buildMessages converts ancestor nodes into LLM messages.
func buildMessages(ancestors []*types.Node) []types.Message {
	var messages []types.Message
	for _, node := range ancestors {
		switch node.NodeType {
		case types.NodeTypeUser:
			messages = append(messages, types.Message{
				Role:    "user",
				Content: json.RawMessage(fmt.Sprintf("%q", node.Content)),
			})
		case types.NodeTypeAssistant:
			messages = append(messages, types.Message{
				Role:    "assistant",
				Content: json.RawMessage(fmt.Sprintf("%q", node.Content)),
			})
		case types.NodeTypeToolResult:
			messages = append(messages, types.Message{
				Role:    "user",
				Content: json.RawMessage(fmt.Sprintf("%q", node.Content)),
			})
		}
	}
	return messages
}

// ResolveNode finds a node by exact ID or prefix.
func (m *Manager) ResolveNode(ctx context.Context, idOrPrefix string) (*types.Node, error) {
	node, err := m.storage.GetNode(ctx, idOrPrefix)
	if err != nil {
		return nil, err
	}
	if node != nil {
		return node, nil
	}
	return m.storage.GetNodeByPrefix(ctx, idOrPrefix)
}

// ListRoots returns all root nodes.
func (m *Manager) ListRoots(ctx context.Context) ([]*types.Node, error) {
	return m.storage.ListRootNodes(ctx)
}

// GetSubtree returns a node and all its descendants.
func (m *Manager) GetSubtree(ctx context.Context, nodeID string) ([]*types.Node, error) {
	return m.storage.GetSubtree(ctx, nodeID)
}

// DeleteNode deletes a node and its subtree.
func (m *Manager) DeleteNode(ctx context.Context, id string) error {
	return m.storage.DeleteNode(ctx, id)
}

// UpdateTitle updates the title on a root node.
func (m *Manager) UpdateTitle(ctx context.Context, nodeID, title string) error {
	node, err := m.storage.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return fmt.Errorf("node not found: %s", nodeID)
	}
	node.Title = title
	return m.storage.UpdateNode(ctx, node)
}

// GenerateTitle generates a title from the first message.
func GenerateTitle(text string) string {
	if len(text) > 50 {
		return text[:47] + "..."
	}
	return text
}
