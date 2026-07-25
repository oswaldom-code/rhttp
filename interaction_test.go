package rhttp_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestTimeoutOuterRetry_TotalBudget(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, syscall.ECONNREFUSED
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(
			rhttp.Timeout(300*time.Millisecond),
			rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts: 3,
				Backoff:     rhttp.ConstantBackoff(200 * time.Millisecond),
			}),
		),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded from the outer timeout, got %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("expected the 300ms budget to cut the run at 2 attempts, got %d", got)
	}
}

func TestRetryOuterTimeout_PerAttempt(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(150 * time.Millisecond):
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
		}
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(
			rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts: 3,
				Backoff:     rhttp.ConstantBackoff(time.Millisecond),
			}),
			rhttp.Timeout(100*time.Millisecond),
		),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, each with a fresh per-attempt deadline, got %d", got)
	}
}

func TestRetryOuterCircuitBreaker_OpenCutsAttempts(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, syscall.ECONNREFUSED
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(
			rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts: 5,
				Backoff:     rhttp.ConstantBackoff(time.Millisecond),
			}),
			rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
				FailureThreshold: 2,
				ResetTimeout:     time.Hour,
			}),
		),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	// This pins today's behavior: ErrCircuitOpen is not retryable, so the
	// tripped breaker short-circuits the remaining retry budget.
	if !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("expected the transport to see only the 2 attempts that tripped the breaker, got %d", got)
	}
}
