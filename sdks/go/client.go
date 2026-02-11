package langdag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the LangDAG API client.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	apiKey      string
	bearerToken string
}

// Option is a function that configures the Client.
type Option func(*Client)

// NewClient creates a new LangDAG API client.
func NewClient(baseURL string, opts ...Option) *Client {
	baseURL = strings.TrimSuffix(baseURL, "/")

	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) Option {
	return func(c *Client) {
		c.apiKey = key
	}
}

// WithBearerToken sets the bearer token for authentication.
func WithBearerToken(token string) Option {
	return func(c *Client) {
		c.bearerToken = token
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// Health checks the server health.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.doRequest(ctx, http.MethodGet, "/health", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Prompt starts a new conversation tree with the given message.
func (c *Client) Prompt(ctx context.Context, message string, opts ...PromptOption) (*Node, error) {
	o := &promptOptions{}
	for _, opt := range opts {
		opt(o)
	}

	req := promptRequest{
		Message:      message,
		Model:        o.model,
		SystemPrompt: o.systemPrompt,
	}

	var resp promptResponse
	if err := c.doRequest(ctx, http.MethodPost, "/prompt", req, &resp); err != nil {
		return nil, err
	}

	return &Node{
		ID:      resp.NodeID,
		Content: resp.Content,
		Type:    NodeTypeAssistant,
		client:  c,
	}, nil
}

// PromptStream starts a new conversation tree with streaming.
func (c *Client) PromptStream(ctx context.Context, message string, opts ...PromptOption) (*Stream, error) {
	o := &promptOptions{}
	for _, opt := range opts {
		opt(o)
	}

	req := promptRequest{
		Message:      message,
		Model:        o.model,
		SystemPrompt: o.systemPrompt,
		Stream:       true,
	}

	return c.doStreamRequest(ctx, http.MethodPost, "/prompt", req)
}

// promptFrom continues a conversation from an existing node (non-streaming).
func (c *Client) promptFrom(ctx context.Context, nodeID, message string, o *promptOptions) (*Node, error) {
	req := promptRequest{
		Message: message,
		Model:   o.model,
	}

	var resp promptResponse
	if err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/prompt", nodeID), req, &resp); err != nil {
		return nil, err
	}

	return &Node{
		ID:      resp.NodeID,
		Content: resp.Content,
		Type:    NodeTypeAssistant,
		client:  c,
	}, nil
}

// promptStreamFrom continues a conversation from an existing node with streaming.
func (c *Client) promptStreamFrom(ctx context.Context, nodeID, message string, o *promptOptions) (*Stream, error) {
	req := promptRequest{
		Message: message,
		Model:   o.model,
		Stream:  true,
	}

	return c.doStreamRequest(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/prompt", nodeID), req)
}

// GetNode retrieves a single node by ID.
func (c *Client) GetNode(ctx context.Context, id string) (*Node, error) {
	var node Node
	if err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s", id), nil, &node); err != nil {
		return nil, err
	}
	node.client = c
	return &node, nil
}

// GetTree retrieves a node and its full subtree.
func (c *Client) GetTree(ctx context.Context, id string) (*Tree, error) {
	var nodes []Node
	if err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/tree", id), nil, &nodes); err != nil {
		return nil, err
	}
	for i := range nodes {
		nodes[i].client = c
	}
	return &Tree{Nodes: nodes}, nil
}

// ListRoots returns all root nodes (conversation trees).
func (c *Client) ListRoots(ctx context.Context) ([]Node, error) {
	var nodes []Node
	if err := c.doRequest(ctx, http.MethodGet, "/nodes", nil, &nodes); err != nil {
		return nil, err
	}
	for i := range nodes {
		nodes[i].client = c
	}
	return nodes, nil
}

// DeleteNode deletes a node and its subtree.
func (c *Client) DeleteNode(ctx context.Context, id string) error {
	return c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/nodes/%s", id), nil, nil)
}

// doRequest performs an HTTP request and decodes the JSON response.
func (c *Client) doRequest(ctx context.Context, method, path string, body, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("langdag: failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("langdag: failed to create request: %w", err)
	}

	c.setHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &ConnectionError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.parseError(resp)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("langdag: failed to decode response: %w", err)
		}
	}

	return nil
}

// doStreamRequest performs an HTTP request and returns a Stream for SSE events.
func (c *Client) doStreamRequest(ctx context.Context, method, path string, body interface{}) (*Stream, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("langdag: failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("langdag: failed to create request: %w", err)
	}

	c.setHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "text/event-stream")

	// Use a client without timeout for streaming
	client := &http.Client{
		Transport: c.httpClient.Transport,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, &ConnectionError{Err: err}
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, c.parseError(resp)
	}

	return newStream(resp.Body, c), nil
}

// setHeaders sets common headers on a request.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
}

// parseError parses an error response from the API.
func (c *Client) parseError(resp *http.Response) error {
	var errResp struct {
		Error string `json:"error"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		errResp.Error = string(body)
		if errResp.Error == "" {
			errResp.Error = resp.Status
		}
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    errResp.Error,
	}
}
