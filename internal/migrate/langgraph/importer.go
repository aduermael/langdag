package langgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"langdag.com/langdag/internal/storage"
	"langdag.com/langdag/types"
)

// ImportOptions configures the import behavior.
type ImportOptions struct {
	// SkipExisting skips threads already in the target storage (by matching
	// metadata.original_thread_id). Note: this requires scanning all root nodes.
	SkipExisting bool
	// DryRun logs what would be imported without actually writing.
	DryRun bool
	// Progress is called with (thread index, total threads, thread_id) after
	// each thread is processed.
	Progress func(i, total int, threadID string)
}

// Result contains statistics from an import operation.
type Result struct {
	ThreadsImported  int
	ThreadsSkipped   int
	MessagesImported int
	Errors           []error
}

// toolCallContent is the JSON structure stored in NodeTypeToolCall content.
type toolCallContent struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

// nodeMetadata is added to every imported node.
type nodeMetadata struct {
	Source            string          `json:"source"`
	OriginalThreadID  string          `json:"original_thread_id"`
	OriginalMessageID string          `json:"original_message_id,omitempty"`
	ThreadMetadata    json.RawMessage `json:"thread_metadata,omitempty"`
}

// ImportFromFile reads a JSON export file and imports it into the given storage.
func ImportFromFile(ctx context.Context, path string, store storage.Storage, opts ImportOptions) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("langgraph import: failed to open file %q: %w", path, err)
	}
	defer f.Close()

	var data ExportData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("langgraph import: failed to parse JSON from %q: %w", path, err)
	}

	return ImportExportData(ctx, &data, store, opts)
}

// ImportExportData imports ExportData into the given storage.
func ImportExportData(ctx context.Context, data *ExportData, store storage.Storage, opts ImportOptions) (*Result, error) {
	result := &Result{}
	total := len(data.Threads)

	// Build a set of already-imported thread IDs if SkipExisting is set.
	existingThreadIDs := map[string]bool{}
	if opts.SkipExisting {
		roots, err := store.ListRootNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("langgraph import: failed to list existing roots: %w", err)
		}
		for _, root := range roots {
			tid := extractOriginalThreadID(root.Metadata)
			if tid != "" {
				existingThreadIDs[tid] = true
			}
		}
	}

	for i, thread := range data.Threads {
		if opts.SkipExisting && existingThreadIDs[thread.ThreadID] {
			result.ThreadsSkipped++
			if opts.Progress != nil {
				opts.Progress(i+1, total, thread.ThreadID)
			}
			continue
		}

		n, err := importThread(ctx, &thread, data.ExportedAt, store, opts.DryRun)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("thread %s: %w", thread.ThreadID, err))
		} else {
			result.ThreadsImported++
			result.MessagesImported += n
		}

		if opts.Progress != nil {
			opts.Progress(i+1, total, thread.ThreadID)
		}
	}

	return result, nil
}

// importThread converts a single ExportThread into a tree of langdag Nodes and saves them.
// Returns the number of nodes created.
func importThread(ctx context.Context, thread *ExportThread, exportedAt time.Time, store storage.Storage, dryRun bool) (int, error) {
	if len(thread.Messages) == 0 {
		return 0, nil
	}

	// Separate system messages from the rest; collect them as system prompt.
	var systemPrompt string
	var messages []ExportMessage
	for _, m := range thread.Messages {
		if m.Role == "system" {
			if systemPrompt != "" {
				systemPrompt += "\n\n"
			}
			systemPrompt += m.Content
		} else {
			messages = append(messages, m)
		}
	}

	if len(messages) == 0 {
		return 0, nil
	}

	// Ensure timestamps are ordered. If messages share identical or zero timestamps
	// we space them 1 microsecond apart to preserve insertion order.
	timestamps := resolveTimestamps(messages, exportedAt)

	// Build the node tree.
	// We track the "current parent" as we walk the linear message chain.
	// Tool calls branch off the assistant node; tool results attach to the tool call node.

	var nodes []*types.Node
	sequence := 0

	// toolCallNodeByID maps tool_call_id → node ID so tool result messages can
	// find their parent tool call node.
	toolCallNodeByID := map[string]string{}
	// lastNodeID is the ID of the last non-tool-result node, used as fallback parent.
	lastNodeID := ""

	for i, msg := range messages {
		ts := timestamps[i]

		meta := nodeMetadata{
			Source:            "langgraph",
			OriginalThreadID:  thread.ThreadID,
			OriginalMessageID: msg.ID,
			ThreadMetadata:    thread.Metadata,
		}
		// Only embed thread metadata on the root node (first message).
		if i != 0 {
			meta.ThreadMetadata = nil
		}
		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal node metadata: %w", err)
		}

		switch msg.Role {
		case "user":
			node := &types.Node{
				ID:        uuid.New().String(),
				Sequence:  sequence,
				NodeType:  types.NodeTypeUser,
				Content:   msg.Content,
				CreatedAt: ts,
				Metadata:  json.RawMessage(metaBytes),
			}
			if i == 0 {
				// Root node: no parent, set title, optionally system prompt.
				node.ParentID = ""
				node.Title = titleFromContent(msg.Content)
				node.SystemPrompt = systemPrompt
			} else {
				node.ParentID = lastNodeID
			}
			nodes = append(nodes, node)
			lastNodeID = node.ID
			sequence++

		case "assistant":
			node := &types.Node{
				ID:        uuid.New().String(),
				ParentID:  lastNodeID,
				Sequence:  sequence,
				NodeType:  types.NodeTypeAssistant,
				Content:   msg.Content,
				Model:     msg.Model,
				TokensIn:  msg.TokensIn,
				TokensOut: msg.TokensOut,
				CreatedAt: ts,
				Metadata:  json.RawMessage(metaBytes),
			}
			nodes = append(nodes, node)
			lastNodeID = node.ID
			sequence++

			// Create child nodes for each tool call.
			for _, tc := range msg.ToolCalls {
				tcContent := toolCallContent{
					Name:  tc.Name,
					Input: tc.Input,
				}
				tcContentBytes, err := json.Marshal(tcContent)
				if err != nil {
					return 0, fmt.Errorf("failed to marshal tool call content: %w", err)
				}

				tcMeta := nodeMetadata{
					Source:            "langgraph",
					OriginalThreadID:  thread.ThreadID,
					OriginalMessageID: tc.ID,
				}
				tcMetaBytes, _ := json.Marshal(tcMeta)

				tcNode := &types.Node{
					ID:        uuid.New().String(),
					ParentID:  node.ID,
					Sequence:  sequence,
					NodeType:  types.NodeTypeToolCall,
					Content:   string(tcContentBytes),
					CreatedAt: ts.Add(time.Microsecond),
					Metadata:  json.RawMessage(tcMetaBytes),
				}
				nodes = append(nodes, tcNode)
				sequence++

				// Map the tool_call_id to this node's ID so tool results can find it.
				if tc.ID != "" {
					toolCallNodeByID[tc.ID] = tcNode.ID
				}
				// Update lastNodeID to the last tool call node so tool results
				// can attach to it if tool_call_id is not set.
				lastNodeID = tcNode.ID
			}

		case "tool":
			// Find the parent tool call node.
			parentID := lastNodeID
			if msg.ToolCallID != "" {
				if tcNodeID, ok := toolCallNodeByID[msg.ToolCallID]; ok {
					parentID = tcNodeID
				}
			}

			node := &types.Node{
				ID:        uuid.New().String(),
				ParentID:  parentID,
				Sequence:  sequence,
				NodeType:  types.NodeTypeToolResult,
				Content:   msg.Content,
				CreatedAt: ts,
				Metadata:  json.RawMessage(metaBytes),
			}
			nodes = append(nodes, node)
			lastNodeID = node.ID
			sequence++

		default:
			// Unknown role — skip.
			continue
		}
	}

	if len(nodes) == 0 {
		return 0, nil
	}

	// Set root_id on all nodes (root is always the first node).
	rootID := nodes[0].ID
	for _, node := range nodes {
		node.RootID = rootID
	}

	if dryRun {
		return len(nodes), nil
	}

	// Persist nodes in order (parent before child).
	for _, node := range nodes {
		if err := store.CreateNode(ctx, node); err != nil {
			return 0, fmt.Errorf("failed to create node %s: %w", node.ID, err)
		}
	}

	return len(nodes), nil
}

// resolveTimestamps returns a timestamp for each message in the slice.
// If a message has a CreatedAt, it is used; otherwise the fallback (export time) is used.
// If two adjacent messages have the same timestamp, the later one is nudged by 1µs to
// preserve ordering.
func resolveTimestamps(messages []ExportMessage, fallback time.Time) []time.Time {
	ts := make([]time.Time, len(messages))
	for i, m := range messages {
		if m.CreatedAt != nil && !m.CreatedAt.IsZero() {
			ts[i] = m.CreatedAt.UTC()
		} else {
			ts[i] = fallback.UTC()
		}
	}
	// Ensure strictly increasing order.
	for i := 1; i < len(ts); i++ {
		if !ts[i].After(ts[i-1]) {
			ts[i] = ts[i-1].Add(time.Microsecond)
		}
	}
	return ts
}

// titleFromContent returns the first ~50 characters of content as a title.
func titleFromContent(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= 50 {
		return content
	}
	// Try to break at a word boundary.
	truncated := content[:50]
	if idx := strings.LastIndexByte(truncated, ' '); idx > 30 {
		truncated = truncated[:idx]
	}
	return truncated + "..."
}

// extractOriginalThreadID reads the original_thread_id from a node's metadata JSON.
func extractOriginalThreadID(metadata json.RawMessage) string {
	if len(metadata) == 0 {
		return ""
	}
	var m struct {
		OriginalThreadID string `json:"original_thread_id"`
	}
	if err := json.Unmarshal(metadata, &m); err != nil {
		return ""
	}
	return m.OriginalThreadID
}
