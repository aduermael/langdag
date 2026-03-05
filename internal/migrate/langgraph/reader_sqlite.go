package langgraph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteReader reads LangGraph checkpoint data from a SQLite database.
type SQLiteReader struct {
	db *sql.DB
}

// NewSQLiteReader opens a LangGraph SQLite database for reading.
func NewSQLiteReader(path string) (*SQLiteReader, error) {
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("langgraph sqlite: failed to open database: %w", err)
	}
	// Verify we can connect and that the expected tables exist.
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='checkpoints'`).Scan(&tableName)
	if err == sql.ErrNoRows {
		db.Close()
		return nil, fmt.Errorf("langgraph sqlite: 'checkpoints' table not found; is this a LangGraph database?")
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("langgraph sqlite: failed to verify schema: %w", err)
	}
	return &SQLiteReader{db: db}, nil
}

// Close closes the database connection.
func (r *SQLiteReader) Close() error {
	return r.db.Close()
}

// ReadExportData reads all threads from the SQLite database and returns ExportData.
// It finds the latest checkpoint per thread (root namespace only) and decodes the
// msgpack-encoded checkpoint blob to extract messages.
func (r *SQLiteReader) ReadExportData(ctx context.Context) (*ExportData, error) {
	exportedAt := time.Now().UTC()

	// Get all distinct thread IDs in the root namespace.
	threadRows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT thread_id
		FROM checkpoints
		WHERE checkpoint_ns = ''
		ORDER BY thread_id
	`)
	if err != nil {
		return nil, fmt.Errorf("langgraph sqlite: failed to query thread IDs: %w", err)
	}
	defer threadRows.Close()

	var threadIDs []string
	for threadRows.Next() {
		var tid string
		if err := threadRows.Scan(&tid); err != nil {
			return nil, fmt.Errorf("langgraph sqlite: failed to scan thread_id: %w", err)
		}
		threadIDs = append(threadIDs, tid)
	}
	if err := threadRows.Err(); err != nil {
		return nil, fmt.Errorf("langgraph sqlite: thread query error: %w", err)
	}
	threadRows.Close()

	var threads []ExportThread
	for _, tid := range threadIDs {
		thread, err := r.readThread(ctx, tid, exportedAt)
		if err != nil {
			// Log and skip problematic threads rather than aborting entire import.
			// Caller can inspect errors via Result.Errors if needed.
			continue
		}
		if thread != nil {
			threads = append(threads, *thread)
		}
	}

	return &ExportData{
		Version:    "1",
		SourceType: "langgraph",
		ExportedAt: exportedAt,
		Threads:    threads,
	}, nil
}

// readThread reads the latest checkpoint for a thread and extracts messages.
func (r *SQLiteReader) readThread(ctx context.Context, threadID string, exportedAt time.Time) (*ExportThread, error) {
	// Get the latest checkpoint for this thread. LangGraph uses UUID v6 checkpoint IDs
	// which sort lexicographically by time, so MAX(checkpoint_id) gives the latest.
	var checkpointBlob []byte
	var metadataBlob []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT checkpoint, metadata
		FROM checkpoints
		WHERE thread_id = ? AND checkpoint_ns = ''
		ORDER BY checkpoint_id DESC
		LIMIT 1
	`, threadID).Scan(&checkpointBlob, &metadataBlob)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("langgraph sqlite: failed to read checkpoint for thread %s: %w", threadID, err)
	}

	messages, err := extractMessagesFromCheckpoint(checkpointBlob)
	if err != nil {
		return nil, fmt.Errorf("langgraph sqlite: failed to extract messages from thread %s: %w", threadID, err)
	}

	thread := &ExportThread{
		ThreadID: threadID,
		Messages: messages,
	}

	// Attempt to parse thread metadata.
	if len(metadataBlob) > 0 {
		meta, err := decodeMetadataBlob(metadataBlob)
		if err == nil && len(meta) > 0 {
			thread.Metadata = meta
		}
	}

	// Set thread created_at from the first message if available.
	for _, m := range messages {
		if m.CreatedAt != nil {
			t := *m.CreatedAt
			thread.CreatedAt = &t
			break
		}
	}

	return thread, nil
}

// extractMessagesFromCheckpoint decodes a msgpack checkpoint blob and extracts messages.
func extractMessagesFromCheckpoint(blob []byte) ([]ExportMessage, error) {
	if len(blob) == 0 {
		return nil, nil
	}

	decoded, err := decodeMsgpack(blob)
	if err != nil {
		// If msgpack fails, try JSON (some older LangGraph versions may use JSON).
		var jsonData map[string]interface{}
		if jsonErr := json.Unmarshal(blob, &jsonData); jsonErr != nil {
			return nil, fmt.Errorf("failed to decode checkpoint as msgpack (%v) or JSON (%v)", err, jsonErr)
		}
		decoded = jsonData
	}

	top, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("checkpoint is not a map")
	}

	// Navigate: checkpoint["channel_values"]["messages"]
	channelValues := msgpackGetMap(top, "channel_values")
	if channelValues == nil {
		// Some checkpoints may store data differently; return empty.
		return nil, nil
	}

	rawMessages := msgpackGetSlice(channelValues, "messages")
	if rawMessages == nil {
		return nil, nil
	}

	var messages []ExportMessage
	for _, rawMsg := range rawMessages {
		msg, ok := rawMsg.(map[string]interface{})
		if !ok {
			continue
		}
		exportMsg, err := extractLangChainMessage(msg)
		if err != nil {
			// Skip unparseable messages but don't abort.
			continue
		}
		messages = append(messages, exportMsg)
	}
	return messages, nil
}

// extractLangChainMessage converts a decoded LangChain message map to ExportMessage.
// Handles both LangChain serialization formats:
//
//	Format 1 (LangChain serializable):
//	  {"lc": 1, "type": "constructor", "id": [...], "kwargs": {"content": ..., ...}}
//
//	Format 2 (direct dict):
//	  {"type": "human", "content": "...", "id": "..."}
func extractLangChainMessage(msg map[string]interface{}) (ExportMessage, error) {
	var em ExportMessage

	// Detect format by presence of "lc" key.
	if lcVal, hasLC := msg["lc"]; hasLC {
		// Format 1: LangChain serializable object.
		_ = lcVal
		kwargs := msgpackGetMap(msg, "kwargs")
		if kwargs == nil {
			// Try direct fields if kwargs missing.
			kwargs = msg
		}

		// Determine role from the class id path.
		// e.g. ["langchain_core", "messages", "human", "HumanMessage"]
		role := lcMessageTypeToRole(msg, kwargs)
		em.Role = role

		em.ID = msgpackGetString(kwargs, "id")
		em.Content = extractLangChainContent(kwargs)

		// Try to get model from response_metadata.
		if respMeta := msgpackGetMap(kwargs, "response_metadata"); respMeta != nil {
			em.Model = msgpackGetString(respMeta, "model")
			if em.Model == "" {
				em.Model = msgpackGetString(respMeta, "model_name")
			}
		}

		// Token usage from usage_metadata.
		if usageMeta := msgpackGetMap(kwargs, "usage_metadata"); usageMeta != nil {
			em.TokensIn = msgpackGetInt(usageMeta, "input_tokens")
			em.TokensOut = msgpackGetInt(usageMeta, "output_tokens")
		}

		// Tool calls.
		em.ToolCalls = extractLangChainToolCalls(kwargs)

		// Tool call ID for tool messages.
		em.ToolCallID = msgpackGetString(kwargs, "tool_call_id")
		em.ToolName = msgpackGetString(kwargs, "name")

	} else {
		// Format 2: Direct dict.
		msgType := msgpackGetString(msg, "type")
		em.Role = langChainTypeToRole(msgType)
		em.ID = msgpackGetString(msg, "id")
		em.Content = extractLangChainContentDirect(msg)
		em.Model = msgpackGetString(msg, "model")

		if usageMeta := msgpackGetMap(msg, "usage_metadata"); usageMeta != nil {
			em.TokensIn = msgpackGetInt(usageMeta, "input_tokens")
			em.TokensOut = msgpackGetInt(usageMeta, "output_tokens")
		}

		em.ToolCalls = extractLangChainToolCalls(msg)
		em.ToolCallID = msgpackGetString(msg, "tool_call_id")
		em.ToolName = msgpackGetString(msg, "name")
	}

	if em.Role == "" {
		return em, fmt.Errorf("could not determine message role")
	}

	return em, nil
}

// lcMessageTypeToRole derives the role string from a LangChain format-1 message.
func lcMessageTypeToRole(msg, kwargs map[string]interface{}) string {
	// Try "type" key in kwargs first (LangChain often sets this).
	if t := msgpackGetString(kwargs, "type"); t != "" {
		return langChainTypeToRole(t)
	}

	// Try to infer from the class id array: e.g. [..., "HumanMessage"].
	if idVal, ok := msg["id"]; ok {
		if idArr, ok := idVal.([]interface{}); ok && len(idArr) > 0 {
			last, _ := idArr[len(idArr)-1].(string)
			return langChainClassToRole(last)
		}
	}

	// Try "type" in the top-level message itself (may be "constructor").
	return ""
}

// langChainTypeToRole converts a LangChain message type string to a role string.
func langChainTypeToRole(t string) string {
	switch t {
	case "human", "HumanMessage":
		return "user"
	case "ai", "AIMessage", "assistant":
		return "assistant"
	case "tool", "ToolMessage":
		return "tool"
	case "system", "SystemMessage":
		return "system"
	case "function", "FunctionMessage":
		return "tool"
	default:
		return t
	}
}

// langChainClassToRole converts a LangChain class name to a role string.
func langChainClassToRole(className string) string {
	switch className {
	case "HumanMessage":
		return "user"
	case "AIMessage", "AIMessageChunk":
		return "assistant"
	case "ToolMessage":
		return "tool"
	case "SystemMessage":
		return "system"
	case "FunctionMessage":
		return "tool"
	default:
		return ""
	}
}

// extractLangChainContent extracts text content from a LangChain kwargs map.
// Content may be a string or a list of content blocks.
func extractLangChainContent(kwargs map[string]interface{}) string {
	return extractLangChainContentDirect(kwargs)
}

// extractLangChainContentDirect extracts content from a map that has a "content" key.
func extractLangChainContentDirect(m map[string]interface{}) string {
	v, ok := m["content"]
	if !ok {
		return ""
	}
	switch c := v.(type) {
	case string:
		return c
	case []interface{}:
		// List of content blocks — extract text blocks.
		var parts []string
		for _, block := range c {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			blockType := msgpackGetString(blockMap, "type")
			if blockType == "text" {
				if text := msgpackGetString(blockMap, "text"); text != "" {
					parts = append(parts, text)
				}
			}
		}
		result := ""
		for i, p := range parts {
			if i > 0 {
				result += "\n"
			}
			result += p
		}
		return result
	default:
		return fmt.Sprintf("%v", v)
	}
}

// extractLangChainToolCalls extracts tool calls from a kwargs/message map.
func extractLangChainToolCalls(m map[string]interface{}) []ExportToolCall {
	rawCalls := msgpackGetSlice(m, "tool_calls")
	if rawCalls == nil {
		return nil
	}

	var calls []ExportToolCall
	for _, rawCall := range rawCalls {
		callMap, ok := rawCall.(map[string]interface{})
		if !ok {
			continue
		}
		call := ExportToolCall{
			ID:   msgpackGetString(callMap, "id"),
			Name: msgpackGetString(callMap, "name"),
		}
		// Extract the args/input field.
		if argsVal, ok := callMap["args"]; ok {
			if inputBytes, err := json.Marshal(argsVal); err == nil {
				call.Input = json.RawMessage(inputBytes)
			}
		} else if inputVal, ok := callMap["input"]; ok {
			if inputBytes, err := json.Marshal(inputVal); err == nil {
				call.Input = json.RawMessage(inputBytes)
			}
		}
		calls = append(calls, call)
	}
	return calls
}

// decodeMetadataBlob attempts to decode a LangGraph metadata blob (msgpack or JSON)
// and returns it as a json.RawMessage.
func decodeMetadataBlob(blob []byte) (json.RawMessage, error) {
	if len(blob) == 0 {
		return nil, nil
	}

	decoded, err := decodeMsgpack(blob)
	if err != nil {
		// Try JSON fallback.
		var jsonData interface{}
		if jsonErr := json.Unmarshal(blob, &jsonData); jsonErr != nil {
			return nil, fmt.Errorf("failed to decode metadata as msgpack or JSON")
		}
		decoded = jsonData
	}

	b, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode metadata as JSON: %w", err)
	}
	return json.RawMessage(b), nil
}
