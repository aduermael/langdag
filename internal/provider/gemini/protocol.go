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
	"strings"

	"langdag.com/langdag/types"
)

// --- Request types ---

type geminiRequest struct {
	Contents         []content         `json:"contents"`
	SystemInstruction *content         `json:"system_instruction,omitempty"`
	Tools            []geminiTool      `json:"tools,omitempty"`
	GenerationConfig *generationConfig `json:"generation_config,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *inlineData       `json:"inlineData,omitempty"`
	FileData         *fileData         `json:"fileData,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
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
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type functionResponse struct {
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

type generationConfig struct {
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	StopSequences   []string `json:"stop_sequences,omitempty"`
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
	if hasConfig {
		gr.GenerationConfig = gc
	}

	body, _ := json.Marshal(gr)
	return body
}

func convertMessages(messages []types.Message) []content {
	var result []content

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
				c.Parts = append(c.Parts, part{
					FunctionCall: &functionCall{Name: block.Name, Args: args},
				})
			case "tool_result":
				c.Parts = append(c.Parts, part{
					FunctionResponse: &functionResponse{
						Name:     block.ToolUseID,
						Response: map[string]interface{}{"result": block.Content},
					},
				})
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

// --- Response conversion ---

func convertResponse(resp *geminiResponse) *types.CompletionResponse {
	cr := &types.CompletionResponse{}

	if len(resp.Candidates) > 0 {
		cand := resp.Candidates[0]
		cr.StopReason = strings.ToLower(cand.FinishReason)

		for _, p := range cand.Content.Parts {
			if p.Text != "" {
				cr.Content = append(cr.Content, types.ContentBlock{
					Type: "text",
					Text: p.Text,
				})
			}
			if p.FunctionCall != nil {
				args, _ := json.Marshal(p.FunctionCall.Args)
				cr.Content = append(cr.Content, types.ContentBlock{
					Type:  "tool_use",
					ID:    p.FunctionCall.Name,
					Name:  p.FunctionCall.Name,
					Input: args,
				})
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
			if p.FunctionCall != nil {
				args, _ := json.Marshal(p.FunctionCall.Args)
				block := types.ContentBlock{
					Type:  "tool_use",
					ID:    p.FunctionCall.Name,
					Name:  p.FunctionCall.Name,
					Input: args,
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
		return nil, fmt.Errorf("gemini: API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp.Body, nil
}
