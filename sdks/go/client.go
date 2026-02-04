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
	baseURL    string
	httpClient *http.Client
	apiKey     string
	bearerToken string
}

// Option is a function that configures the Client.
type Option func(*Client)

// NewClient creates a new LangDAG API client.
func NewClient(baseURL string, opts ...Option) *Client {
	// Remove trailing slash from baseURL
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

// doStreamRequest performs an HTTP request and handles SSE streaming.
func (c *Client) doStreamRequest(ctx context.Context, method, path string, body interface{}, handler SSEEventHandler) error {
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
	req.Header.Set("Accept", "text/event-stream")

	// Use a client without timeout for streaming
	client := &http.Client{
		Transport: c.httpClient.Transport,
	}

	resp, err := client.Do(req)
	if err != nil {
		return &ConnectionError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.parseError(resp)
	}

	// Check if response is SSE
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		// Non-streaming response, parse as JSON
		var chatResp ChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			return fmt.Errorf("langdag: failed to decode response: %w", err)
		}
		// Emit synthetic events
		if err := handler(SSEEvent{Type: SSEEventStart, DAGID: chatResp.DAGID}); err != nil {
			return err
		}
		if err := handler(SSEEvent{Type: SSEEventDelta, Content: chatResp.Content}); err != nil {
			return err
		}
		return handler(SSEEvent{Type: SSEEventDone, DAGID: chatResp.DAGID, NodeID: chatResp.NodeID})
	}

	return parseSSEStream(resp.Body, handler)
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
		// If we can't parse the error, use the raw body
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
