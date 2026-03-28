// Package conversation provides conversation management logic.
package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"langdag.com/langdag/internal/provider"
	"langdag.com/langdag/internal/storage"
	"langdag.com/langdag/types"
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
func (m *Manager) Prompt(ctx context.Context, message, model, systemPrompt string, tools []types.ToolDefinition, think *bool, maxTokens int) (<-chan types.StreamEvent, error) {
	rootID := uuid.New().String()
	rootNode := &types.Node{
		ID:           rootID,
		RootID:       rootID,
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
		{Role: "user", Content: contentToRawMessage(message)},
	}

	return m.streamResponse(ctx, rootNode, messages, model, systemPrompt, tools, think, maxTokens)
}

// PromptFrom continues a conversation from an existing node.
// It creates a user child node, builds message history by walking to the root,
// sends to the LLM, and streams the response.
func (m *Manager) PromptFrom(ctx context.Context, parentNodeID, message, model string, tools []types.ToolDefinition, think *bool, maxTokens int) (<-chan types.StreamEvent, error) {
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
		RootID:    root.ID,
		Sequence:  lastNode.Sequence + 1,
		NodeType:  types.NodeTypeUser,
		Content:   message,
		Status:    "completed",
		CreatedAt: time.Now(),
	}
	if err := m.storage.CreateNode(ctx, userNode); err != nil {
		return nil, fmt.Errorf("failed to create user node: %w", err)
	}

	// Index any tool_result IDs in the new user message so future queries
	// can detect orphaned tool_use blocks without parsing JSON content.
	if resultIDs := extractToolResultIDsFromContent(message); len(resultIDs) > 0 {
		_ = m.storage.IndexToolIDs(ctx, userNode.ID, resultIDs, "result")
	}

	// Fix orphaned tool_use blocks: query the DB index (not message JSON)
	// for tool_use IDs among ancestors that have no matching tool_result.
	// This is O(orphans), not O(messages).
	ancestorIDs := make([]string, len(ancestors)+1)
	for i, a := range ancestors {
		ancestorIDs[i] = a.ID
	}
	ancestorIDs[len(ancestors)] = userNode.ID
	orphans, err := m.storage.GetOrphanedToolUses(ctx, ancestorIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to check orphaned tool uses: %w", err)
	}
	if len(orphans) > 0 {
		ancestors = injectSyntheticToolResults(ancestors, orphans)
	}

	// Build message history from ancestors + this new message.
	// If the last message is already "user" (e.g. parent is a tool_result node),
	// merge into that message to maintain role alternation.
	messages := buildMessages(ancestors)
	newContent := contentToRawMessage(message)
	if n := len(messages); n > 0 && messages[n-1].Role == "user" {
		messages[n-1].Content = mergeContent(messages[n-1].Content, newContent)
	} else {
		messages = append(messages, types.Message{
			Role:    "user",
			Content: newContent,
		})
	}

	return m.streamResponse(ctx, userNode, messages, model, root.SystemPrompt, tools, think, maxTokens)
}

// injectSyntheticToolResults inserts synthetic tool_result nodes into the
// ancestor list for any orphaned tool_use blocks. The synthetic nodes are
// not persisted — they exist only for message construction.
func injectSyntheticToolResults(ancestors []*types.Node, orphans map[string][]string) []*types.Node {
	var result []*types.Node
	for _, node := range ancestors {
		result = append(result, node)
		toolIDs, ok := orphans[node.ID]
		if !ok || len(toolIDs) == 0 {
			continue
		}
		// Build synthetic tool_result content for orphaned IDs.
		var blocks []map[string]interface{}
		for _, id := range toolIDs {
			blocks = append(blocks, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": id,
				"content":     "Tool call was not completed.",
				"is_error":    true,
			})
		}
		content, _ := json.Marshal(blocks)
		result = append(result, &types.Node{
			NodeType: types.NodeTypeToolResult,
			Content:  string(content),
			Sequence: node.Sequence, // same sequence for ordering
		})
	}
	return result
}

// extractToolResultIDsFromContent extracts tool_result tool_use_id values
// from a content string (used at write time for DB indexing).
func extractToolResultIDsFromContent(content string) []string {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 || trimmed[0] != '[' || !json.Valid([]byte(trimmed)) {
		return nil
	}
	var blocks []struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
	}
	if json.Unmarshal([]byte(trimmed), &blocks) != nil {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID != "" {
			ids = append(ids, b.ToolUseID)
		}
	}
	return ids
}

// streamResponse sends messages to the LLM and wraps the provider events,
// saving the assistant node when the stream completes.
func (m *Manager) streamResponse(ctx context.Context, parentNode *types.Node, messages []types.Message, model, systemPrompt string, tools []types.ToolDefinition, think *bool, maxTokens int) (<-chan types.StreamEvent, error) {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	req := &types.CompletionRequest{
		Model:     model,
		Messages:  messages,
		System:    systemPrompt,
		MaxTokens: maxTokens,
		Tools:     tools,
		Think:     think,
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
			// Determine the content to store: if the response contains
			// non-text content blocks (e.g. tool_use), store the full
			// JSON-encoded content blocks so callers can parse them.
			nodeContent := fullText
			if response != nil && hasNonTextBlocks(response.Content) {
				if encoded, err := json.Marshal(response.Content); err == nil {
					nodeContent = string(encoded)
				}
			}

			assistantNode := &types.Node{
				ID:        uuid.New().String(),
				ParentID:  parentNode.ID,
				RootID:    parentNode.RootID,
				Sequence:  parentNode.Sequence + 1,
				NodeType:  types.NodeTypeAssistant,
				Content:   nodeContent,
				Model:     model,
				Status:    "completed",
				LatencyMs: int(time.Since(startTime).Milliseconds()),
				CreatedAt: time.Now(),
			}
			if response != nil {
				assistantNode.Provider = response.Provider
				assistantNode.TokensIn = response.Usage.InputTokens
				assistantNode.TokensOut = response.Usage.OutputTokens
				assistantNode.TokensCacheRead = response.Usage.CacheReadInputTokens
				assistantNode.TokensCacheCreation = response.Usage.CacheCreationInputTokens
				assistantNode.TokensReasoning = response.Usage.ReasoningTokens
			}
			if err := m.storage.CreateNode(ctx, assistantNode); err != nil {
				events <- types.StreamEvent{
					Type:  types.StreamEventError,
					Error: fmt.Errorf("failed to save assistant node: %w", err),
				}
				return
			}
			// Index tool_use IDs so orphan detection uses DB queries, not JSON parsing.
			if response != nil {
				var toolUseIDs []string
				for _, block := range response.Content {
					if block.Type == "tool_use" && block.ID != "" {
						toolUseIDs = append(toolUseIDs, block.ID)
					}
				}
				if len(toolUseIDs) > 0 {
					_ = m.storage.IndexToolIDs(ctx, assistantNode.ID, toolUseIDs, "use")
				}
			}
			events <- types.StreamEvent{
				Type:   types.StreamEventNodeSaved,
				NodeID: assistantNode.ID,
			}
		}
	}()

	return events, nil
}

// buildMessages converts ancestor nodes into LLM messages.
// It ensures messages alternate between user and assistant roles by merging
// consecutive same-role messages into a single message with a content block
// array, which prevents API errors from providers that enforce strict
// role alternation (e.g. Anthropic).
func buildMessages(ancestors []*types.Node) []types.Message {
	var messages []types.Message
	for _, node := range ancestors {
		var role string
		switch node.NodeType {
		case types.NodeTypeUser:
			role = "user"
		case types.NodeTypeAssistant:
			role = "assistant"
		case types.NodeTypeToolResult:
			role = "user"
		default:
			continue
		}

		raw := contentToRawMessage(node.Content)

		// If the last message has the same role, merge content into
		// a single JSON array of content blocks to maintain role alternation.
		if n := len(messages); n > 0 && messages[n-1].Role == role {
			messages[n-1].Content = mergeContent(messages[n-1].Content, raw)
			continue
		}

		messages = append(messages, types.Message{
			Role:    role,
			Content: raw,
		})
	}
	return messages
}

// mergeContent combines two json.RawMessage values into a single JSON array
// of content blocks. Each input may be a JSON string or a JSON array.
func mergeContent(a, b json.RawMessage) json.RawMessage {
	blocksA := toContentBlockArray(a)
	blocksB := toContentBlockArray(b)
	merged := append(blocksA, blocksB...)
	out, err := json.Marshal(merged)
	if err != nil {
		// Fallback: concatenate as text blocks (should never happen).
		return a
	}
	return json.RawMessage(out)
}

// toContentBlockArray converts a json.RawMessage that is either a JSON string
// or a JSON array into a []json.RawMessage. A string is wrapped as a text block.
func toContentBlockArray(raw json.RawMessage) []json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil {
			return arr
		}
	}
	// It's a JSON string — wrap as a text content block, but skip empty text
	// to avoid producing {"type":"text","text":""} which the Anthropic API rejects.
	var text string
	if json.Unmarshal(raw, &text) != nil {
		text = string(raw)
	}
	if text == "" {
		return nil
	}
	block, _ := json.Marshal(map[string]string{"type": "text", "text": text})
	return []json.RawMessage{json.RawMessage(block)}
}

// contentToRawMessage converts a content string to a json.RawMessage.
// If the content is a valid JSON array (e.g. content blocks like tool_use or
// tool_result), it is passed through as-is. Otherwise it is encoded as a JSON string.
func contentToRawMessage(content string) json.RawMessage {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) > 0 && trimmed[0] == '[' && json.Valid([]byte(trimmed)) {
		return json.RawMessage(trimmed)
	}
	return json.RawMessage(fmt.Sprintf("%q", content))
}

// ResolveNode finds a node by exact ID, prefix match, or alias.
func (m *Manager) ResolveNode(ctx context.Context, idOrPrefix string) (*types.Node, error) {
	// Try exact ID
	node, err := m.storage.GetNode(ctx, idOrPrefix)
	if err != nil {
		return nil, err
	}
	if node != nil {
		return node, nil
	}

	// Try prefix match
	node, err = m.storage.GetNodeByPrefix(ctx, idOrPrefix)
	if err != nil {
		return nil, err
	}
	if node != nil {
		return node, nil
	}

	// Try alias
	return m.storage.GetNodeByAlias(ctx, idOrPrefix)
}

// CreateAlias creates an alias for a node.
func (m *Manager) CreateAlias(ctx context.Context, nodeID, alias string) error {
	return m.storage.CreateAlias(ctx, nodeID, alias)
}

// DeleteAlias removes an alias.
func (m *Manager) DeleteAlias(ctx context.Context, alias string) error {
	return m.storage.DeleteAlias(ctx, alias)
}

// ListAliases returns all aliases for a node.
func (m *Manager) ListAliases(ctx context.Context, nodeID string) ([]string, error) {
	return m.storage.ListAliases(ctx, nodeID)
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

// hasNonTextBlocks returns true if content contains any non-text blocks (e.g. tool_use).
func hasNonTextBlocks(blocks []types.ContentBlock) bool {
	for _, b := range blocks {
		if b.Type != "text" {
			return true
		}
	}
	return false
}

// GenerateTitle generates a title from the first message.
func GenerateTitle(text string) string {
	if len(text) > 50 {
		return text[:47] + "..."
	}
	return text
}
