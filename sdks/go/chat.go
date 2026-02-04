package langdag

import (
	"context"
	"fmt"
	"net/http"
)

// Chat starts a new conversation and sends the first message.
// If the request has Stream=true, the handler will receive SSE events.
// If the request has Stream=false or handler is nil, this returns a ChatResponse.
func (c *Client) Chat(ctx context.Context, req *NewChatRequest, handler SSEEventHandler) (*ChatResponse, error) {
	if req.Stream && handler != nil {
		if err := c.doStreamRequest(ctx, http.MethodPost, "/chat", req, handler); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Non-streaming request
	req.Stream = false
	var resp ChatResponse
	if err := c.doRequest(ctx, http.MethodPost, "/chat", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ChatStream starts a new conversation with streaming enabled.
// This is a convenience method that sets Stream=true and requires a handler.
func (c *Client) ChatStream(ctx context.Context, req *NewChatRequest, handler SSEEventHandler) error {
	if handler == nil {
		return fmt.Errorf("langdag: handler is required for streaming")
	}
	req.Stream = true
	return c.doStreamRequest(ctx, http.MethodPost, "/chat", req, handler)
}

// ContinueChat continues an existing conversation by appending a new message.
// The dagID can be a full UUID or a prefix (minimum 4 characters).
// If the request has Stream=true, the handler will receive SSE events.
func (c *Client) ContinueChat(ctx context.Context, dagID string, req *ContinueChatRequest, handler SSEEventHandler) (*ChatResponse, error) {
	if req.Stream && handler != nil {
		if err := c.doStreamRequest(ctx, http.MethodPost, fmt.Sprintf("/chat/%s", dagID), req, handler); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Non-streaming request
	req.Stream = false
	var resp ChatResponse
	if err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/chat/%s", dagID), req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ContinueChatStream continues a conversation with streaming enabled.
// This is a convenience method that sets Stream=true and requires a handler.
func (c *Client) ContinueChatStream(ctx context.Context, dagID string, req *ContinueChatRequest, handler SSEEventHandler) error {
	if handler == nil {
		return fmt.Errorf("langdag: handler is required for streaming")
	}
	req.Stream = true
	return c.doStreamRequest(ctx, http.MethodPost, fmt.Sprintf("/chat/%s", dagID), req, handler)
}

// ForkChat forks a conversation from a specific node, creating a new branch.
// The dagID can be a full UUID or a prefix (minimum 4 characters).
// If the request has Stream=true, the handler will receive SSE events.
func (c *Client) ForkChat(ctx context.Context, dagID string, req *ForkChatRequest, handler SSEEventHandler) (*ChatResponse, error) {
	if req.Stream && handler != nil {
		if err := c.doStreamRequest(ctx, http.MethodPost, fmt.Sprintf("/chat/%s/fork", dagID), req, handler); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Non-streaming request
	req.Stream = false
	var resp ChatResponse
	if err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/chat/%s/fork", dagID), req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ForkChatStream forks a conversation with streaming enabled.
// This is a convenience method that sets Stream=true and requires a handler.
func (c *Client) ForkChatStream(ctx context.Context, dagID string, req *ForkChatRequest, handler SSEEventHandler) error {
	if handler == nil {
		return fmt.Errorf("langdag: handler is required for streaming")
	}
	req.Stream = true
	return c.doStreamRequest(ctx, http.MethodPost, fmt.Sprintf("/chat/%s/fork", dagID), req, handler)
}
