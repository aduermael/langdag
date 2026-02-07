package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
)

// generateMessageID generates a mock message ID.
func generateMessageID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 24)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("msg_%s", string(b))
}

// countInputTokens estimates input tokens from the request (rough word count).
func countInputTokens(req *MessagesRequest) int {
	total := 0
	for _, msg := range req.Messages {
		text := extractTextContent(msg.Content)
		total += countWords(text)
	}

	// Count system prompt tokens
	if len(req.System) > 0 {
		var s string
		if err := json.Unmarshal(req.System, &s); err == nil {
			total += countWords(s)
		}
	}

	if total == 0 {
		total = 10
	}
	return total
}

// countWords counts words in a string (rough token estimate).
func countWords(s string) int {
	words := strings.Fields(s)
	if len(words) == 0 {
		return 1
	}
	return len(words)
}

// chunkText splits text into chunks of approximately n words each.
func chunkText(text string, wordsPerChunk int) []string {
	if wordsPerChunk <= 0 {
		wordsPerChunk = 1
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var chunks []string
	for i := 0; i < len(words); i += wordsPerChunk {
		end := i + wordsPerChunk
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[i:end], " ")
		if i+wordsPerChunk < len(words) {
			chunk += " "
		}
		chunks = append(chunks, chunk)
	}

	return chunks
}
