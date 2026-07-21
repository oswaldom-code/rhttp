package rhttp_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
	"github.com/oswaldom-code/rhttp/internal"
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

func TestTokenBucket_Wait(t *testing.T) {
	tb := rhttp.NewTokenBucket(100, 1) // 100 req/s, burst of 1

	// Consume the token
	tb.TryAcquire()

	start := time.Now()
	err := tb.Wait()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have waited ~10ms
	if elapsed < 5*time.Millisecond {
		t.Errorf("expected to wait at least 5ms, waited %v", elapsed)
	}
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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

func TestPerHostRateLimiter(t *testing.T) {
	phl := rhttp.NewPerHostRateLimiter(10, 5)

	limiter1 := phl.GetLimiter("api.example.com")
	limiter2 := phl.GetLimiter("api.other.com")
	limiter3 := phl.GetLimiter("api.example.com") // same as limiter1

	if limiter1 == limiter2 {
		t.Error("expected different limiters for different hosts")
	}

	if limiter1 != limiter3 {
		t.Error("expected same limiter for same host")
	}

	// Drain limiter1
	for i := 0; i < 5; i++ {
		limiter1.TryAcquire()
	}

	// limiter2 should still have tokens
	if !limiter2.TryAcquire() {
		t.Error("expected limiter2 to have tokens")
	}

	// limiter1 should be empty
	if limiter1.TryAcquire() {
		t.Error("expected limiter1 to be empty")
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
