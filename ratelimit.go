package rhttp

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter controls the rate of HTTP requests.
type RateLimiter interface {
	// Wait blocks until a token is available.
	// Deprecated: Use WaitContext for proper cancellation support.
	Wait() error

	// WaitContext blocks until a token is available or context is canceled.
	// Returns an error if the context is canceled.
	WaitContext(ctx context.Context) error

	// TryAcquire attempts to acquire a token without blocking.
	// Returns true if a token was acquired, false otherwise.
	TryAcquire() bool
}

// TokenBucket implements a token bucket rate limiter.
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewTokenBucket creates a new token bucket rate limiter.
// rate: requests per second allowed
// burst: maximum burst size (bucket capacity)
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	return &TokenBucket{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// Wait blocks until a token is available.
// Deprecated: Use WaitContext for proper cancellation support.
func (tb *TokenBucket) Wait() error {
	return tb.WaitContext(context.Background())
}

// WaitContext blocks until a token is available or context is canceled.
func (tb *TokenBucket) WaitContext(ctx context.Context) error {
	for {
		if tb.TryAcquire() {
			return nil
		}

		// Calculate wait time for next token
		tb.mu.Lock()
		waitTime := time.Duration((1.0 / tb.refillRate) * float64(time.Second))
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}
}

// TryAcquire attempts to acquire a token without blocking.
func (tb *TokenBucket) TryAcquire() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now
}

// Tokens returns the current number of available tokens.
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// RateLimitConfig configures the rate limit middleware.
type RateLimitConfig struct {
	// Limiter is the rate limiter to use. Required.
	Limiter RateLimiter

	// WaitOnLimit if true, waits for a token instead of failing immediately.
	// Default is false (fail fast).
	WaitOnLimit bool

	// RespectRetryAfter if true, respects Retry-After header from responses.
	// Default is false.
	RespectRetryAfter bool
}

// RateLimit returns a middleware that applies rate limiting to requests.
func RateLimit(cfg RateLimitConfig) Middleware {
	if cfg.Limiter == nil {
		return func(next http.RoundTripper) http.RoundTripper {
			return next
		}
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return &rateLimitRoundTripper{
			next: next,
			cfg:  cfg,
		}
	}
}

type rateLimitRoundTripper struct {
	next      http.RoundTripper
	cfg       RateLimitConfig
	retryLock sync.Mutex
	retryAt   time.Time
}

func (r *rateLimitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if we're in a Retry-After period
	if r.cfg.RespectRetryAfter {
		r.retryLock.Lock()
		if time.Now().Before(r.retryAt) {
			waitTime := time.Until(r.retryAt)
			r.retryLock.Unlock()

			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(waitTime):
			}
		} else {
			r.retryLock.Unlock()
		}
	}

	// Acquire rate limit token
	if r.cfg.WaitOnLimit {
		if err := r.cfg.Limiter.WaitContext(req.Context()); err != nil {
			return nil, err
		}
	} else if !r.cfg.Limiter.TryAcquire() {
		return nil, ErrRateLimited
	}

	resp, err := r.next.RoundTrip(req)

	// Handle Retry-After header
	if r.cfg.RespectRetryAfter && resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				r.retryLock.Lock()
				r.retryAt = time.Now().Add(time.Duration(seconds) * time.Second)
				r.retryLock.Unlock()
			} else if t, err := http.ParseTime(retryAfter); err == nil {
				r.retryLock.Lock()
				r.retryAt = t
				r.retryLock.Unlock()
			}
		}
	}

	return resp, err
}

// PerHostRateLimiter provides separate rate limiters for each host.
// It lazily creates a TokenBucket for each unique host on first access.
// This is useful when making requests to multiple APIs with different rate limits.
type PerHostRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*TokenBucket
	rate     float64
	burst    int
}

// NewPerHostRateLimiter creates a rate limiter that applies limits per host.
func NewPerHostRateLimiter(rate float64, burst int) *PerHostRateLimiter {
	return &PerHostRateLimiter{
		limiters: make(map[string]*TokenBucket),
		rate:     rate,
		burst:    burst,
	}
}

// GetLimiter returns the rate limiter for a specific host.
// If no limiter exists for the host, a new one is created with the configured
// rate and burst values. This method is safe for concurrent use.
func (p *PerHostRateLimiter) GetLimiter(host string) *TokenBucket {
	p.mu.RLock()
	limiter, ok := p.limiters[host]
	p.mu.RUnlock()

	if ok {
		return limiter
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, ok = p.limiters[host]; ok {
		return limiter
	}

	limiter = NewTokenBucket(p.rate, p.burst)
	p.limiters[host] = limiter
	return limiter
}
