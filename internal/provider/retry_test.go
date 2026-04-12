package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"langdag.com/langdag/types"
)

// failProvider fails N times then succeeds.
type failProvider struct {
	failCount  int
	callCount  int
	failErr    error
}

func (p *failProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	p.callCount++
	if p.callCount <= p.failCount {
		return nil, p.failErr
	}
	return &types.CompletionResponse{Content: []types.ContentBlock{{Type: "text", Text: "ok"}}}, nil
}

func (p *failProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	p.callCount++
	if p.callCount <= p.failCount {
		return nil, p.failErr
	}
	ch := make(chan types.StreamEvent, 1)
	ch <- types.StreamEvent{Type: types.StreamEventDone}
	close(ch)
	return ch, nil
}

func (p *failProvider) Name() string             { return "fail-provider" }
func (p *failProvider) Models() []types.ModelInfo { return nil }

func TestRetryComplete_TransientThenSuccess(t *testing.T) {
	inner := &failProvider{failCount: 2, failErr: fmt.Errorf("status 503: service unavailable")}
	prov := WithRetry(inner, RetryConfig{MaxRetries: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond})

	resp, err := prov.Complete(context.Background(), &types.CompletionRequest{})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if inner.callCount != 3 {
		t.Errorf("callCount = %d, want 3", inner.callCount)
	}
}

func TestRetryComplete_MaxRetriesExceeded(t *testing.T) {
	inner := &failProvider{failCount: 5, failErr: fmt.Errorf("status 500: internal server error")}
	prov := WithRetry(inner, RetryConfig{MaxRetries: 2, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond})

	_, err := prov.Complete(context.Background(), &types.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if inner.callCount != 3 { // 1 initial + 2 retries
		t.Errorf("callCount = %d, want 3", inner.callCount)
	}
}

func TestRetryComplete_NonTransientError(t *testing.T) {
	inner := &failProvider{failCount: 5, failErr: fmt.Errorf("status 401: unauthorized")}
	prov := WithRetry(inner, RetryConfig{MaxRetries: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond})

	_, err := prov.Complete(context.Background(), &types.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error for non-transient failure")
	}
	if inner.callCount != 1 {
		t.Errorf("callCount = %d, want 1 (no retries for non-transient)", inner.callCount)
	}
}

func TestRetryStream_TransientThenSuccess(t *testing.T) {
	inner := &failProvider{failCount: 1, failErr: fmt.Errorf("status 429: rate limited")}
	prov := WithRetry(inner, RetryConfig{MaxRetries: 2, BaseDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond})

	ch, err := prov.Stream(context.Background(), &types.CompletionRequest{})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	if inner.callCount != 2 {
		t.Errorf("callCount = %d, want 2", inner.callCount)
	}
}

func TestRetryComplete_ContextCancelled(t *testing.T) {
	inner := &failProvider{failCount: 10, failErr: fmt.Errorf("status 503: unavailable")}
	prov := WithRetry(inner, RetryConfig{MaxRetries: 5, BaseDelay: 100 * time.Millisecond, MaxDelay: 1 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := prov.Complete(ctx, &types.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRetryZeroRetries(t *testing.T) {
	inner := &failProvider{failCount: 0}
	prov := WithRetry(inner, RetryConfig{MaxRetries: 0})

	// Should return inner directly (no wrapping)
	if _, ok := prov.(*retryProvider); ok {
		t.Error("expected unwrapped provider when MaxRetries=0")
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		err       error
		transient bool
	}{
		{fmt.Errorf("status 500: internal error"), true},
		{fmt.Errorf("status 502: bad gateway"), true},
		{fmt.Errorf("status 503: service unavailable"), true},
		{fmt.Errorf("status 429: rate limit exceeded"), true},
		{fmt.Errorf("status 529: overloaded"), true},
		{fmt.Errorf("connection refused"), true},
		{fmt.Errorf("timeout"), true},
		{fmt.Errorf("unexpected EOF"), true},
		{fmt.Errorf("write: broken pipe"), true},
		{fmt.Errorf("TLS handshake timeout"), true},
		{fmt.Errorf("server is overloaded"), true},
		{fmt.Errorf("dial tcp: lookup api.example.com: no such host"), true},
		{fmt.Errorf("status 401: unauthorized"), false},
		{fmt.Errorf("status 400: bad request"), false},
		{fmt.Errorf("invalid model"), false},
		{nil, false},
	}

	for _, tt := range tests {
		got := isTransient(tt.err)
		if got != tt.transient {
			t.Errorf("isTransient(%v) = %v, want %v", tt.err, got, tt.transient)
		}
	}
}

// TestRetryComplete_ContextCancelDuringBackoff verifies that canceling the
// context during the backoff sleep exits immediately, not after the full delay.
func TestRetryComplete_ContextCancelDuringBackoff(t *testing.T) {
	inner := &failProvider{failCount: 100, failErr: fmt.Errorf("status 503: unavailable")}
	prov := WithRetry(inner, RetryConfig{
		MaxRetries: 5,
		BaseDelay:  5 * time.Second, // long backoff
		MaxDelay:   10 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay — during the first backoff sleep.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := prov.Complete(ctx, &types.CompletionRequest{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	// Should exit quickly after cancel, not wait for the full 5s backoff.
	if elapsed > 1*time.Second {
		t.Errorf("took %v — should exit quickly when context canceled during backoff", elapsed)
	}
	// Should have made only 1 call (the initial attempt fails, then backoff
	// is interrupted by context cancellation).
	if inner.callCount != 1 {
		t.Errorf("callCount = %d, want 1 (only the initial attempt)", inner.callCount)
	}
}

// TestRetryStream_ContextCancelDuringBackoff verifies the same behavior for Stream.
func TestRetryStream_ContextCancelDuringBackoff(t *testing.T) {
	inner := &failProvider{failCount: 100, failErr: fmt.Errorf("status 503: unavailable")}
	prov := WithRetry(inner, RetryConfig{
		MaxRetries: 5,
		BaseDelay:  5 * time.Second,
		MaxDelay:   10 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := prov.Stream(ctx, &types.CompletionRequest{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if elapsed > 1*time.Second {
		t.Errorf("took %v — should exit quickly when context canceled during backoff", elapsed)
	}
}

// TestRetryMaxRetries1_ExactAttempts verifies that MaxRetries=1 means
// exactly 1 retry (2 total attempts).
func TestRetryMaxRetries1_ExactAttempts(t *testing.T) {
	inner := &failProvider{failCount: 100, failErr: fmt.Errorf("status 500: error")}
	prov := WithRetry(inner, RetryConfig{
		MaxRetries: 1,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	})

	_, err := prov.Complete(context.Background(), &types.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	// 1 initial + 1 retry = 2 total attempts
	if inner.callCount != 2 {
		t.Errorf("callCount = %d, want 2 (1 initial + 1 retry)", inner.callCount)
	}
}

// TestIsTransient_DeepWrapping verifies that isTransient detects transient
// errors even when wrapped 3+ levels deep with fmt.Errorf.
func TestIsTransient_DeepWrapping(t *testing.T) {
	base := fmt.Errorf("status 503: service unavailable")
	wrapped1 := fmt.Errorf("provider call failed: %w", base)
	wrapped2 := fmt.Errorf("router: %w", wrapped1)
	wrapped3 := fmt.Errorf("agent: %w", wrapped2)

	// isTransient checks the error message string, which includes wrapped messages.
	if !isTransient(wrapped3) {
		t.Error("isTransient should detect 503 through 3 levels of wrapping")
	}

	// Non-transient wrapped deeply
	nonTransient := fmt.Errorf("agent: %w", fmt.Errorf("router: %w", fmt.Errorf("status 401: unauthorized")))
	if isTransient(nonTransient) {
		t.Error("isTransient should not classify 401 as transient even when wrapped")
	}
}

// TestIsTransient_EdgeCaseMessages tests edge-case error strings that
// might or might not match transient patterns.
func TestIsTransient_EdgeCaseMessages(t *testing.T) {
	tests := []struct {
		msg       string
		transient bool
	}{
		// Variants of timeout phrasing
		{"connection timeout", true},        // contains "timeout"
		{"request timed out", false},         // "timed out" does NOT contain "timeout"
		{"context deadline exceeded", false}, // Go stdlib timeout message — not matched by string
		{"temporary failure in name resolution", true}, // contains "temporary failure"

		// Rate limit variants
		{"rate limit exceeded", true},
		{"Rate Limit Exceeded", true}, // case-insensitive match on "rate limit"
		{"you have been rate limited", true},

		// 5xx in unusual positions
		{"error code: 502", true},
		{"HTTP 504 Gateway Timeout", true},

		// Numbers that look like status codes but aren't
		{"invoice #50032 not found", true}, // false positive: contains "500"
		{"port 5003 is busy", true},         // false positive: contains "500"

		// Connection errors
		{"connection reset by peer", true},
		{"connection refused by server", true},

		// EOF variants
		{"unexpected EOF", true},
		{"read: EOF", true},
		{"io.EOF", true}, // "EOF" substring is present

		// Broken pipe
		{"write tcp 127.0.0.1:8080: write: broken pipe", true},

		// TLS errors
		{"TLS handshake error", true},
		{"tls handshake", false}, // case-sensitive — lowercase not matched

		// Overloaded
		{"server is overloaded", true},
		{"Overloaded", true}, // case-insensitive match on "overloaded"

		// DNS — "no such host" is retried (DNS can fail transiently in containers/cloud)
		{"dial tcp: lookup api.example.com: no such host", true},

		// 529 (Anthropic overloaded status)
		{"status 529: overloaded", true},
	}

	for _, tt := range tests {
		got := isTransient(fmt.Errorf("%s", tt.msg))
		if got != tt.transient {
			t.Errorf("isTransient(%q) = %v, want %v", tt.msg, got, tt.transient)
		}
	}
}

// TestRetryComplete_SucceedsOnLastAttempt verifies that retry succeeds
// when the provider recovers on the very last allowed attempt.
func TestRetryComplete_SucceedsOnLastAttempt(t *testing.T) {
	// MaxRetries=3 → 4 total attempts. Fail first 3, succeed on 4th.
	inner := &failProvider{failCount: 3, failErr: fmt.Errorf("status 503: unavailable")}
	prov := WithRetry(inner, RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	})

	resp, err := prov.Complete(context.Background(), &types.CompletionRequest{})
	if err != nil {
		t.Fatalf("expected success on last attempt, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if inner.callCount != 4 {
		t.Errorf("callCount = %d, want 4", inner.callCount)
	}
}
