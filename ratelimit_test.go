package rhttp_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestTokenBucket_Basic(t *testing.T) {
	tb := rhttp.NewTokenBucket(10, 5) // 10 req/s, burst of 5

	// Should be able to acquire 5 tokens immediately (burst)
	for i := 0; i < 5; i++ {
		if !tb.TryAcquire() {
			t.Fatalf("expected to acquire token %d", i)
		}
	}

	// 6th should fail
	if tb.TryAcquire() {
		t.Fatal("expected 6th acquire to fail")
	}
}

func TestNewTokenBucket_ZeroRateIsUnlimited(t *testing.T) {
	tb := rhttp.NewTokenBucket(0, 1)

	// An invalid rate must not limit: without the guard, only the initial burst
	// token is granted and WaitContext then busy-loops on a negative wait time.
	for i := 0; i < 10; i++ {
		if !tb.TryAcquire() {
			t.Fatalf("attempt %d: invalid rate must not limit", i)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := tb.WaitContext(ctx); err != nil {
		t.Fatalf("WaitContext on unlimited bucket returned error: %v", err)
	}
}

func TestNewTokenBucket_ZeroBurstIsUnlimited(t *testing.T) {
	tb := rhttp.NewTokenBucket(10, 0)

	// Zero burst must not block forever (maxTokens == 0 → TryAcquire never true).
	if !tb.TryAcquire() {
		t.Fatal("zero burst must not block forever")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := tb.WaitContext(ctx); err != nil {
		t.Fatalf("WaitContext on unlimited bucket returned error: %v", err)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := rhttp.NewTokenBucket(100, 1) // 100 req/s, burst of 1

	// Consume the token
	if !tb.TryAcquire() {
		t.Fatal("expected to acquire initial token")
	}

	// Should fail immediately
	if tb.TryAcquire() {
		t.Fatal("expected acquire to fail immediately after drain")
	}

	// Wait for refill (10ms for 1 token at 100/s)
	time.Sleep(15 * time.Millisecond)

	// Should succeed after refill
	if !tb.TryAcquire() {
		t.Fatal("expected to acquire token after refill")
	}
}

func TestTokenBucket_WaitContextBlocksUntilToken(t *testing.T) {
	tb := rhttp.NewTokenBucket(100, 1) // 100 req/s, burst of 1

	// Consume the token
	tb.TryAcquire()

	start := time.Now()
	err := tb.WaitContext(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have waited ~10ms
	if elapsed < 5*time.Millisecond {
		t.Errorf("expected to wait at least 5ms, waited %v", elapsed)
	}
}

// stubLimiter mirrors the method set of x/time/rate.Limiter without importing it.
type stubLimiter struct{}

func (stubLimiter) Allow() bool                  { return true }
func (stubLimiter) Wait(_ context.Context) error { return nil }

// xRateAdapter shows that adapting an x/time/rate style limiter to
// rhttp.RateLimiter takes a struct and two one-line methods.
type xRateAdapter struct{ l stubLimiter }

func (a xRateAdapter) TryAcquire() bool                      { return a.l.Allow() }
func (a xRateAdapter) WaitContext(ctx context.Context) error { return a.l.Wait(ctx) }

func TestRateLimiter_XTimeRateAdapter(t *testing.T) {
	var limiter rhttp.RateLimiter = xRateAdapter{}

	rt := rhttp.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{Limiter: limiter})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
}

func TestTokenBucket_Concurrent(t *testing.T) {
	tb := rhttp.NewTokenBucket(1000, 100)

	var acquired int64
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tb.TryAcquire() {
				atomic.AddInt64(&acquired, 1)
			}
		}()
	}

	wg.Wait()

	if acquired != 100 {
		t.Errorf("expected 100 acquired, got %d", acquired)
	}
}

func TestRateLimit_Middleware(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	limiter := rhttp.NewTokenBucket(1000, 10)
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{
			Limiter:     limiter,
			WaitOnLimit: true,
		})),
	)

	// Should succeed within burst
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, err := c.Do(context.Background(), req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}

	if calls != 10 {
		t.Errorf("expected 10 calls, got %d", calls)
	}
}

func TestRateLimit_NoWait(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	limiter := rhttp.NewTokenBucket(1, 1) // 1 req/s, burst of 1
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{
			Limiter:     limiter,
			WaitOnLimit: false,
		})),
	)

	// First should succeed
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	// Second should fail immediately
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err = c.Do(context.Background(), req)
	if !errors.Is(err, rhttp.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestRateLimit_RespectRetryAfter(t *testing.T) {
	callCount := 0
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		if callCount == 1 {
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Request:    req,
			}
			resp.Header.Set("Retry-After", "1") // 1 second
			return resp, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	limiter := rhttp.NewTokenBucket(1000, 100)
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{
			Limiter:           limiter,
			WaitOnLimit:       true,
			RespectRetryAfter: true,
		})),
	)

	// First request gets 429
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, _ := c.Do(context.Background(), req)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}

	// Second request should wait for Retry-After
	start := time.Now()
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, _ = c.Do(context.Background(), req)
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Should have waited ~1 second
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected to wait ~1s for Retry-After, waited %v", elapsed)
	}
}

func TestRateLimit_NilLimiter(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{
			Limiter: nil,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func BenchmarkTokenBucket_TryAcquire(b *testing.B) {
	tb := rhttp.NewTokenBucket(1000000, 1000000) // high limits

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tb.TryAcquire()
	}
}

func BenchmarkTokenBucket_Concurrent(b *testing.B) {
	tb := rhttp.NewTokenBucket(1000000, 1000000)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tb.TryAcquire()
		}
	})
}

func TestRateLimit_FailFastClosesRequestBody(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	limiter := rhttp.NewTokenBucket(1, 1)
	limiter.TryAcquire()

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{Limiter: limiter})),
	)

	rec := &closeRecorder{Reader: strings.NewReader("payload")}
	req, _ := http.NewRequest(http.MethodPut, "http://example.com", http.NoBody)
	req.Body = rec
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, rhttp.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if !rec.closed {
		t.Error("request body was not closed on rate-limit short-circuit")
	}
}

func TestTokenBucket_TokensReportsAvailability(t *testing.T) {
	tb := rhttp.NewTokenBucket(1, 5) // 1 token/s: refill drift is negligible

	if got := tb.Tokens(); got != 5 {
		t.Fatalf("expected a full bucket of 5 tokens, got %v", got)
	}

	tb.TryAcquire()
	tb.TryAcquire()

	got := tb.Tokens()
	if got < 3 || got >= 4 {
		t.Fatalf("expected ~3 tokens after two acquires, got %v", got)
	}
}

func TestTokenBucket_WaitContextAlreadyCanceled(t *testing.T) {
	tb := rhttp.NewTokenBucket(1, 1)
	tb.TryAcquire()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := tb.WaitContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("expected immediate return on canceled ctx, took %v", elapsed)
	}
}

func TestRateLimit_RetryAfterHTTPDate(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Request:    req,
			}
			resp.Header.Set("Retry-After", time.Now().Add(2*time.Second).UTC().Format(http.TimeFormat))
			return resp, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	limiter := rhttp.NewTokenBucket(1000, 100)
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{
			Limiter:           limiter,
			RespectRetryAfter: true,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	start := time.Now()
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("expected to honor the HTTP-date Retry-After (~1-2s), waited %v", elapsed)
	}
}

func TestRateLimit_CtxCanceledDuringRetryAfterWait(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     make(http.Header),
			Request:    req,
		}
		resp.Header.Set("Retry-After", "2")
		return resp, nil
	})

	limiter := rhttp.NewTokenBucket(1000, 100)
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{
			Limiter:           limiter,
			RespectRetryAfter: true,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(ctx, req)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded during Retry-After wait, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("expected the canceled ctx to cut the 2s wait, took %v", elapsed)
	}
}

func TestRateLimit_RespectRetryAfterConcurrent(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&calls, 1)%3 == 0 {
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Request:    req,
			}
			resp.Header.Set("Retry-After", "0")
			return resp, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	limiter := rhttp.NewTokenBucket(100000, 1000)
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.RateLimit(rhttp.RateLimitConfig{
			Limiter:           limiter,
			RespectRetryAfter: true,
		})),
	)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
			_, _ = c.Do(context.Background(), req)
		}()
	}
	wg.Wait()
}

// NewTokenBucket falls back to not limiting on invalid configuration, which is
// indistinguishable from a correctly configured bucket until the load it was
// meant to shape arrives. NewTokenBucketE reports it instead.
func TestNewTokenBucketE_InvalidConfigIsAnError(t *testing.T) {
	tests := []struct {
		name  string
		rate  float64
		burst int
	}{
		{"zero rate", 0, 10},
		{"negative rate", -1, 10},
		{"zero burst", 10, 0},
		{"negative burst", 10, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, err := rhttp.NewTokenBucketE(tt.rate, tt.burst)

			if !errors.Is(err, rhttp.ErrInvalidRateLimit) {
				t.Errorf("error = %v, want it to wrap ErrInvalidRateLimit", err)
			}
			if bucket != nil {
				t.Error("an invalid configuration must not yield a usable bucket")
			}
		})
	}
}

func TestNewTokenBucketE_ValidConfigLimits(t *testing.T) {
	bucket, err := rhttp.NewTokenBucketE(10, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bucket.TryAcquire() {
		t.Error("a valid bucket should admit its first request")
	}
	if bucket.TryAcquire() {
		t.Error("a burst of 1 should admit only one request")
	}
}
