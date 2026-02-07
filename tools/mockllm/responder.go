package main

import (
	"encoding/json"
	"math/rand"
	"strings"
)

// Responder generates mock responses based on the configured mode.
type Responder struct {
	cfg *Config
}

// NewResponder creates a new Responder.
func NewResponder(cfg *Config) *Responder {
	return &Responder{cfg: cfg}
}

// GenerateResponse generates a response based on the configured mode.
func (r *Responder) GenerateResponse(req *MessagesRequest) string {
	switch r.cfg.Mode {
	case "echo":
		return r.echoResponse(req)
	case "fixed":
		return r.fixedResponse()
	case "error":
		return "" // errors are handled at the HTTP level
	default:
		return r.randomResponse()
	}
}

// echoResponse echoes the last user message content.
func (r *Responder) echoResponse(req *MessagesRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role == "user" {
			return extractTextContent(msg.Content)
		}
	}
	return "No user message to echo."
}

// fixedResponse returns the configured fixed response.
func (r *Responder) fixedResponse() string {
	if r.cfg.FixedResponse != "" {
		return r.cfg.FixedResponse
	}
	return "This is a mock response from the LangDAG mock LLM server."
}

// randomResponse generates random lorem ipsum text.
func (r *Responder) randomResponse() string {
	sentences := rand.Intn(5) + 3
	var result []string
	for i := 0; i < sentences; i++ {
		result = append(result, loremSentences[rand.Intn(len(loremSentences))])
	}
	return strings.Join(result, " ")
}

// extractTextContent extracts text from message content (string or []ContentBlock).
func extractTextContent(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}

	return string(raw)
}

var loremSentences = []string{
	"The quick brown fox jumps over the lazy dog.",
	"Lorem ipsum dolor sit amet, consectetur adipiscing elit.",
	"Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
	"Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.",
	"Duis aute irure dolor in reprehenderit in voluptate velit esse cillum.",
	"Excepteur sint occaecat cupidatat non proident, sunt in culpa.",
	"Nemo enim ipsam voluptatem quia voluptas sit aspernatur aut odit.",
	"Neque porro quisquam est, qui dolorem ipsum quia dolor sit amet.",
	"Quis autem vel eum iure reprehenderit qui in ea voluptate velit esse.",
	"At vero eos et accusamus et iusto odio dignissimos ducimus.",
	"Nam libero tempore, cum soluta nobis est eligendi optio cumque nihil impedit.",
	"Temporibus autem quibusdam et aut officiis debitis aut rerum necessitatibus.",
}
