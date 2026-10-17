package rhttp_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

// Allocation gates enforced by the Performance workflow. They fail the build
// when a documented zero-allocation promise or the full-stack ceiling is
// broken. Allocation counts are deterministic, unlike ns/op on shared runners.

// fullStackAllocCeiling is the documented allocs/op of the complete middleware
// stack (see README "Middleware overhead"). Raise it only with a justification
// in the same commit.
const fullStackAllocCeiling = 11

func assertZeroAllocs(t *testing.T, name string, fn func()) {
	t.Helper()
	if allocs := testing.AllocsPerRun(1000, fn); allocs != 0 {
		t.Errorf("%s: got %.0f allocs/op, want 0", name, allocs)
	}
}

func TestAllocs_BackoffStrategies(t *testing.T) {
	base, maxDuration := 100*time.Millisecond, 10*time.Second
	strategies := map[string]rhttp.BackoffFunc{
		"Constant":               rhttp.ConstantBackoff(base),
		"Linear":                 rhttp.LinearBackoff(base, maxDuration),
		"Exponential":            rhttp.ExponentialBackoff(base, maxDuration),
		"Fibonacci":              rhttp.FibonacciBackoff(base, maxDuration),
		"DecorrelatedJitter":     rhttp.DecorrelatedJitterBackoff(base, maxDuration),
		"ExponentialFullJitter":  rhttp.ExponentialBackoffFullJitter(base, maxDuration),
		"ExponentialEqualJitter": rhttp.ExponentialBackoffEqualJitter(base, maxDuration),
		"WithJitter":             rhttp.WithJitter(rhttp.ConstantBackoff(base), 0.2),
		"WithMin":                rhttp.WithMin(rhttp.ConstantBackoff(base), base),
		"WithMax":                rhttp.WithMax(rhttp.ConstantBackoff(base), maxDuration),
	}

	for name, backoff := range strategies {
		assertZeroAllocs(t, name, func() { _ = backoff(3, nil) })
	}
}

func TestAllocs_TokenBucketTryAcquire(t *testing.T) {
	bucket := rhttp.NewTokenBucket(1e9, 1000)
	assertZeroAllocs(t, "TokenBucket.TryAcquire", func() { _ = bucket.TryAcquire() })
}

func TestAllocs_FullMiddlewareStack(t *testing.T) {
	c := rhttp.New(
		rhttp.WithTransport(noopRoundTripper),
		rhttp.WithMiddleware(
			rhttp.Timeout(5*time.Second),
			rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
				FailureThreshold: 5,
				ResetTimeout:     30 * time.Second,
			}),
			rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3}),
			rhttp.Logging(rhttp.LoggingConfig{Logger: noopLogger}),
			rhttp.Metrics(rhttp.MetricsConfig{Recorder: noopRecorder}),
		),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	allocs := testing.AllocsPerRun(1000, func() { _, _ = c.Do(ctx, req) })
	if allocs > fullStackAllocCeiling {
		t.Errorf("full middleware stack: got %.0f allocs/op, ceiling is %d", allocs, fullStackAllocCeiling)
	}
	t.Logf("full middleware stack: %.0f allocs/op (ceiling %d)", allocs, fullStackAllocCeiling)
}
