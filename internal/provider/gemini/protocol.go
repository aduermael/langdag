// Package gemini provides Google Gemini provider implementations.
// This file contains shared request building, response parsing, SSE streaming,
// and message/tool conversion used by all Gemini variants (direct, Vertex).
package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"langdag.com/langdag/types"
)

// apiError represents an API error with optional retry information.
type apiError struct {
	statusCode int
	body       string
	retryAfter time.Duration
}

func (e *apiError) Error() string {
	return fmt.Sprintf("gemini: API error (status %d): %s", e.statusCode, e.body)
}

func (e *apiError) RetryAfter() time.Duration {
	return e.retryAfter
}

// parseRetryDelay extracts the retry delay from a Google API error response
// that contains a google.rpc.RetryInfo detail.
func parseRetryDelay(body []byte) time.Duration {
	var errResp struct {
		Error struct {
			Details []json.RawMessage `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		return 0
	}
	for _, detail := range errResp.Error.Details {
		var retryInfo struct {
			Type       string `json:"@type"`
			RetryDelay string `json:"retryDelay"`
		}
		if err := json.Unmarshal(detail, &retryInfo); err != nil {
			continue
		}
		if retryInfo.Type == "type.googleapis.com/google.rpc.RetryInfo" && retryInfo.RetryDelay != "" {
			// The delay is typically like "21s" or "21.243533579s".
			d, err := time.ParseDuration(retryInfo.RetryDelay)
			if err == nil {
				return d
			}
		}
	}
	return 0
}

// --- Request types ---

type geminiRequest struct {
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"system_instruction,omitempty"`
	Tools             []geminiTool      `json:"tools,omitempty"`
	ToolConfig        *toolConfig       `json:"tool_config,omitempty"`
	GenerationConfig  *generationConfig `json:"generation_config,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	InlineData       *inlineData       `json:"inlineData,omitempty"`
	FileData         *fileData         `json:"fileData,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type fileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

type functionCall struct {
	ID   string                 `json:"id,omitempty"`
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type functionResponse struct {
	ID       string                 `json:"id,omitempty"`
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

// geminiTool represents a single entry in the Gemini tools array.
// It can be either a function declarations bundle or a server-side tool.
type geminiTool struct {
	// For function declarations (mutually exclusive with serverToolName)
	FunctionDeclarations []functionDeclaration `json:"function_declarations,omitempty"`
	// For server-side tools — the tool name (e.g. "google_search", "code_execution")
	serverToolName string
}

// MarshalJSON implements custom marshaling. Server tools are serialized as
// {"<name>": {}} which is the Gemini API format for built-in tools.
func (t geminiTool) MarshalJSON() ([]byte, error) {
	if t.serverToolName != "" {
		return json.Marshal(map[string]interface{}{t.serverToolName: map[string]interface{}{}})
	}
	// For function declarations, use standard marshaling
	type alias struct {
		FunctionDeclarations []functionDeclaration `json:"function_declarations,omitempty"`
	}
	return json.Marshal(alias{FunctionDeclarations: t.FunctionDeclarations})
}

type functionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type thinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget"`
}

type generationConfig struct {
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Temperature     *float64         `json:"temperature,omitempty"`
	StopSequences   []string         `json:"stop_sequences,omitempty"`
	ThinkingConfig  *thinkingConfig  `json:"thinkingConfig,omitempty"`
}

type toolConfig struct {
	IncludeServerSideToolInvocations bool `json:"include_server_side_tool_invocations,omitempty"`
}

// --- Response types ---

type geminiResponse struct {
	Candidates    []candidate    `json:"candidates"`
	UsageMetadata *usageMetadata `json:"usageMetadata,omitempty"`
}

type candidate struct {
	Content      content `json:"content"`
	FinishReason string  `json:"finishReason,omitempty"`
}

type usageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
}

// --- Request building ---

func buildRequest(req *types.CompletionRequest) []byte {
	gr := geminiRequest{
		Contents: convertMessages(req.Messages),
	}

	if req.System != "" {
		gr.SystemInstruction = &content{
			Parts: []part{{Text: req.System}},
		}
	}

	if len(req.Tools) > 0 {
		gr.Tools = convertTools(req.Tools)
		for _, t := range req.Tools {
			if !t.IsClientTool() {
				gr.ToolConfig = &toolConfig{IncludeServerSideToolInvocations: true}
				break
			}
		}
	}

	gc := &generationConfig{}
	hasConfig := false
	if req.MaxTokens > 0 {
		gc.MaxOutputTokens = req.MaxTokens
		hasConfig = true
	}
	if req.Temperature > 0 {
		gc.Temperature = &req.Temperature
		hasConfig = true
	}
	if len(req.StopSeqs) > 0 {
		gc.StopSequences = req.StopSeqs
		hasConfig = true
	}
	if req.Think != nil {
		if *req.Think {
			gc.ThinkingConfig = &thinkingConfig{ThinkingBudget: 8192}
		} else {
			gc.ThinkingConfig = &thinkingConfig{ThinkingBudget: 0}
		}
		hasConfig = true
	}
	if hasConfig {
		gr.GenerationConfig = gc
	}

	body, _ := json.Marshal(gr)
	return body
}

func convertMessages(messages []types.Message) []content {
	var result []content
	// Map of tool call ID → function name, built from tool_use blocks
	// so that tool_result blocks can resolve the function name.
	callNames := make(map[string]string)

	for _, msg := range messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		} else if role != "model" {
			role = "user"
		}

		c := content{Role: role}

		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			c.Parts = []part{{Text: text}}
			result = append(result, c)
			continue
		}

		var blocks []types.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			c.Parts = []part{{Text: string(msg.Content)}}
			result = append(result, c)
			continue
		}

		for _, block := range blocks {
			switch block.Type {
			case "text":
				c.Parts = append(c.Parts, part{Text: block.Text})
			case "image", "document":
				if block.Data != "" {
					c.Parts = append(c.Parts, part{
						InlineData: &inlineData{MimeType: block.MediaType, Data: block.Data},
					})
				} else if block.URL != "" {
					c.Parts = append(c.Parts, part{
						FileData: &fileData{MimeType: block.MediaType, FileURI: block.URL},
					})
				}
			case "tool_use":
				var args map[string]interface{}
				if block.Input != nil {
					json.Unmarshal(block.Input, &args)
				}
				fc := &functionCall{Name: block.Name, Args: args}
				if block.ID != "" && block.ID != block.Name {
					fc.ID = block.ID
				}
				p := part{FunctionCall: fc}
				// Restore thought signature from provider data.
				if len(block.ProviderData) > 0 {
					var pd geminiProviderData
					if json.Unmarshal(block.ProviderData, &pd) == nil && pd.ThoughtSignature != "" {
						p.ThoughtSignature = pd.ThoughtSignature
					}
				}
				c.Parts = append(c.Parts, p)
				// Track ID → name for tool_result resolution.
				if block.ID != "" {
					callNames[block.ID] = block.Name
				}
			case "tool_result":
				name := block.ToolUseID
				if n, ok := callNames[block.ToolUseID]; ok {
					name = n
				}
				fr := &functionResponse{
					Name:     name,
					Response: map[string]interface{}{"result": block.Content},
				}
				// Include ID when it differs from the name (Gemini 3.x).
				if block.ToolUseID != name {
					fr.ID = block.ToolUseID
				}
				c.Parts = append(c.Parts, part{FunctionResponse: fr})
			}
		}

		if len(c.Parts) > 0 {
			result = append(result, c)
		}
	}

	return result
}

// geminiServerTools maps standardized tool names to Gemini-specific tool names.
var geminiServerTools = map[string]string{
	types.ServerToolWebSearch: "google_search",
}

func convertTools(tools []types.ToolDefinition) []geminiTool {
	var decls []functionDeclaration
	var serverTools []geminiTool

	for _, t := range tools {
		if t.IsClientTool() {
			decls = append(decls, functionDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			})
			continue
		}

		// Server-side tool: map name if known, otherwise pass through as-is
		toolName := t.Name
		if mapped, ok := geminiServerTools[t.Name]; ok {
			toolName = mapped
		}
		serverTools = append(serverTools, geminiTool{serverToolName: toolName})
	}

	var result []geminiTool
	if len(decls) > 0 {
		result = append(result, geminiTool{FunctionDeclarations: decls})
	}
	result = append(result, serverTools...)
	return result
}

// geminiProviderData holds Gemini-specific data stored in ContentBlock.ProviderData
// to survive round-trips (e.g. thought signatures for multi-turn tool use).
type geminiProviderData struct {
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

// --- Response conversion ---

func convertResponse(resp *geminiResponse) *types.CompletionResponse {
	cr := &types.CompletionResponse{}

	if len(resp.Candidates) > 0 {
		cand := resp.Candidates[0]
		cr.StopReason = strings.ToLower(cand.FinishReason)

		for _, p := range cand.Content.Parts {
			if p.Thought {
				continue
			}
			if p.Text != "" {
				cr.Content = append(cr.Content, types.ContentBlock{
					Type: "text",
					Text: p.Text,
				})
			}
			if p.FunctionCall != nil {
				args, _ := json.Marshal(p.FunctionCall.Args)
				block := types.ContentBlock{
					Type:  "tool_use",
					ID:    p.FunctionCall.Name,
					Name:  p.FunctionCall.Name,
					Input: args,
				}
				if p.FunctionCall.ID != "" {
					block.ID = p.FunctionCall.ID
				}
				if p.ThoughtSignature != "" {
					block.ProviderData, _ = json.Marshal(geminiProviderData{
						ThoughtSignature: p.ThoughtSignature,
					})
				}
				cr.Content = append(cr.Content, block)
			}
		}
	}

	if resp.UsageMetadata != nil {
		cr.Usage = mapUsage(resp.UsageMetadata)
	}

	return cr
}

func mapUsage(u *usageMetadata) types.Usage {
	return types.Usage{
		InputTokens:          u.PromptTokenCount,
		OutputTokens:         u.CandidatesTokenCount,
		CacheReadInputTokens: u.CachedContentTokenCount,
		ReasoningTokens:      u.ThoughtsTokenCount,
	}
}

// --- SSE streaming ---

func parseSSEStream(body io.Reader, events chan<- types.StreamEvent) {
	events <- types.StreamEvent{Type: types.StreamEventStart}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var prevText string
	var lastUsage *types.Usage
	var toolUseBlocks []types.ContentBlock

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var resp geminiResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

		if resp.UsageMetadata != nil {
			u := mapUsage(resp.UsageMetadata)
			lastUsage = &u
		}

		if len(resp.Candidates) == 0 {
			continue
		}

		cand := resp.Candidates[0]

		var currentText string
		for _, p := range cand.Content.Parts {
			if p.Thought {
				continue
			}
			if p.Text != "" {
				currentText += p.Text
			}
		}

		if len(currentText) > len(prevText) {
			delta := currentText[len(prevText):]
			events <- types.StreamEvent{
				Type:    types.StreamEventDelta,
				Content: delta,
			}
			prevText = currentText
		}

		for _, p := range cand.Content.Parts {
			if p.Thought {
				continue
			}
			if p.FunctionCall != nil {
				args, _ := json.Marshal(p.FunctionCall.Args)
				block := types.ContentBlock{
					Type:  "tool_use",
					ID:    p.FunctionCall.Name,
					Name:  p.FunctionCall.Name,
					Input: args,
				}
				if p.FunctionCall.ID != "" {
					block.ID = p.FunctionCall.ID
				}
				if p.ThoughtSignature != "" {
					block.ProviderData, _ = json.Marshal(geminiProviderData{
						ThoughtSignature: p.ThoughtSignature,
					})
				}
				toolUseBlocks = append(toolUseBlocks, block)
				events <- types.StreamEvent{
					Type:         types.StreamEventContentDone,
					ContentBlock: &block,
				}
			}
		}
	}

	resp := &types.CompletionResponse{}
	if lastUsage != nil {
		resp.Usage = *lastUsage
	}
	if prevText != "" {
		resp.Content = append(resp.Content, types.ContentBlock{
			Type: "text",
			Text: prevText,
		})
	}
	resp.Content = append(resp.Content, toolUseBlocks...)

	events <- types.StreamEvent{
		Type:     types.StreamEventDone,
		Response: resp,
	}
}

// doHTTPRequest performs an HTTP POST and returns the response body.
func doHTTPRequest(ctx context.Context, client *http.Client, url string, body []byte, headers map[string]string) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)

		apiErr := &apiError{
			statusCode: resp.StatusCode,
			body:       string(bodyBytes),
		}

		// Parse retry delay from Retry-After header.
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				apiErr.retryAfter = time.Duration(secs) * time.Second
			}
		}

		// For rate limit errors, also check the Google RetryInfo in the JSON body.
		if resp.StatusCode == http.StatusTooManyRequests && apiErr.retryAfter == 0 {
			apiErr.retryAfter = parseRetryDelay(bodyBytes)
		}

		return nil, apiErr
	}

	return resp.Body, nil
}
