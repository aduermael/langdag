package provider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"strings"
	"time"

	"langdag.com/langdag/types"
)

// retryCallbackKey is the context key for per-call retry callbacks.
type retryCallbackKey struct{}

// ContextWithRetryCallback returns a child context that carries a per-call
// retry callback. When a retryProvider fires a retry, it checks the context
// first; this takes priority over the config-level OnRetry.
func ContextWithRetryCallback(ctx context.Context, fn func(RetryEvent)) context.Context {
	return context.WithValue(ctx, retryCallbackKey{}, fn)
}

// RetryAfterError is implemented by errors that carry a server-suggested retry delay.
type RetryAfterError interface {
	error
	RetryAfter() time.Duration
}

// RetryEvent holds information about a retry attempt, passed to OnRetry callbacks.
type RetryEvent struct {
	Err        error
	Attempt    int
	MaxRetries int
	Delay      time.Duration
}

// RetryConfig configures retry behavior for provider calls.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	// OnRetry is called before each retry wait. It may be nil.
	OnRetry func(RetryEvent)
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
	}
}

// retryProvider wraps a Provider with retry logic.
type retryProvider struct {
	inner  Provider
	config RetryConfig
}

// WithRetry wraps a Provider with exponential backoff retry logic.
// Only transient errors (5xx, rate limits, timeouts) are retried.
func WithRetry(p Provider, cfg RetryConfig) Provider {
	if cfg.MaxRetries <= 0 {
		return p
	}
	return &retryProvider{inner: p, config: cfg}
}

func (r *retryProvider) Name() string          { return r.inner.Name() }
func (r *retryProvider) Models() []types.ModelInfo { return r.inner.Models() }

func (r *retryProvider) Complete(ctx context.Context, req *types.CompletionRequest) (*types.CompletionResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := r.retryDelay(attempt, lastErr)
			r.notifyRetry(ctx, lastErr, attempt, delay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := r.inner.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}

		if !isTransient(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (r *retryProvider) Stream(ctx context.Context, req *types.CompletionRequest) (<-chan types.StreamEvent, error) {
	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := r.retryDelay(attempt, lastErr)
			r.notifyRetry(ctx, lastErr, attempt, delay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ch, err := r.inner.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}

		if !isTransient(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// notifyRetry calls the per-call context callback, the config-level OnRetry,
// or falls back to log.Printf.
func (r *retryProvider) notifyRetry(ctx context.Context, err error, attempt int, delay time.Duration) {
	ev := RetryEvent{
		Err:        err,
		Attempt:    attempt,
		MaxRetries: r.config.MaxRetries,
		Delay:      delay,
	}
	if fn, ok := ctx.Value(retryCallbackKey{}).(func(RetryEvent)); ok && fn != nil {
		fn(ev)
	} else if r.config.OnRetry != nil {
		r.config.OnRetry(ev)
	} else {
		log.Printf("Retry %d/%d after %v: %v", attempt, r.config.MaxRetries, delay, err)
	}
}

// retryDelay returns the delay before a retry attempt. If the error carries a
// server-suggested retry delay (RetryAfterError), that value is used with small
// jitter. Otherwise, exponential backoff is used.
func (r *retryProvider) retryDelay(attempt int, lastErr error) time.Duration {
	var rae RetryAfterError
	if errors.As(lastErr, &rae) {
		if d := rae.RetryAfter(); d > 0 {
			// Small jitter on server-suggested delay (1.0x to 1.1x).
			jitter := 1.0 + 0.1*rand.Float64()
			return time.Duration(float64(d) * jitter)
		}
	}
	return r.backoff(attempt)
}

// backoff calculates the delay for a given retry attempt using exponential backoff with jitter.
func (r *retryProvider) backoff(attempt int) time.Duration {
	delay := float64(r.config.BaseDelay) * math.Pow(2, float64(attempt-1))
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}
	// Add jitter: 0.5x to 1.5x
	jitter := 0.5 + rand.Float64()
	return time.Duration(delay * jitter)
}

// isTransient returns true if the error is likely transient and worth retrying.
func isTransient(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	// Rate limit errors
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") {
		return true
	}

	// Server errors (5xx)
	for _, code := range []string{"500", "502", "503", "504"} {
		if strings.Contains(msg, code) {
			return true
		}
	}

	// Network errors
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary failure") {
		return true
	}

	// Check for net.Error (timeout, temporary)
	var netErr net.Error
	if ok := errorAs(err, &netErr); ok {
		return netErr.Timeout()
	}

	return false
}

// errorAs is a helper that wraps errors.As for net.Error.
func errorAs(err error, target *net.Error) bool {
	for err != nil {
		if ne, ok := err.(net.Error); ok {
			*target = ne
			return true
		}
		if unwrapper, ok := err.(interface{ Unwrap() error }); ok {
			err = unwrapper.Unwrap()
		} else {
			return false
		}
	}
	return false
}
