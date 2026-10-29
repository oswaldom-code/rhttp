package rhttp

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter controls the rate of HTTP requests.
type RateLimiter interface {
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
	waitTime   time.Duration // time for one token to refill; immutable
	unlimited  bool
}

// NewTokenBucket creates a new token bucket rate limiter.
// rate: requests per second allowed
// burst: maximum burst size (bucket capacity)
//
// A non-positive rate or a burst below 1 cannot produce a limiter. Rather than
// busy-loop or block permanently, the returned bucket falls back to not
// limiting: it allows every request. That fallback is reported through
// OnInvalidConfig, and NewTokenBucketE returns it as an error instead.
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	if rate <= 0 || burst < 1 {
		reportInvalidConfig("TokenBucket", fmt.Sprintf(
			"rate=%v burst=%d: rate must be positive and burst at least 1; the bucket does not limit",
			rate, burst))
		return &TokenBucket{unlimited: true}
	}
	return &TokenBucket{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: rate,
		lastRefill: time.Now(),
		waitTime:   time.Duration(float64(time.Second) / rate),
	}
}

// NewTokenBucketE is NewTokenBucket with the invalid cases reported instead of
// silently disabled. Prefer it whenever the rate comes from configuration that
// could be wrong: a bucket that does not limit is indistinguishable from a
// correctly configured one until the load it was meant to shape arrives.
//
// The returned error wraps ErrInvalidRateLimit.
func NewTokenBucketE(rate float64, burst int) (*TokenBucket, error) {
	if rate <= 0 || burst < 1 {
		return nil, fmt.Errorf("%w: rate=%v burst=%d: rate must be positive and burst at least 1",
			ErrInvalidRateLimit, rate, burst)
	}
	return NewTokenBucket(rate, burst), nil
}

// WaitContext blocks until a token is available or context is canceled.
func (tb *TokenBucket) WaitContext(ctx context.Context) error {
	for {
		if tb.TryAcquire() {
			return nil
		}

		timer := time.NewTimer(tb.waitTime)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// TryAcquire attempts to acquire a token without blocking.
func (tb *TokenBucket) TryAcquire() bool {
	if tb.unlimited {
		return true
	}

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
//
// A nil Limiter cannot rate-limit anything, so the middleware falls back to a
// pass-through. That fallback is reported through OnInvalidConfig.
func RateLimit(cfg RateLimitConfig) Middleware {
	if cfg.Limiter == nil {
		reportInvalidConfig("RateLimit", "Limiter is nil: requests are not rate-limited")
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

			timer := time.NewTimer(waitTime)
			select {
			case <-req.Context().Done():
				timer.Stop()
				closeRequestBody(req)
				return nil, req.Context().Err()
			case <-timer.C:
			}
		} else {
			r.retryLock.Unlock()
		}
	}

	// Acquire rate limit token
	if r.cfg.WaitOnLimit {
		if err := r.cfg.Limiter.WaitContext(req.Context()); err != nil {
			closeRequestBody(req)
			return nil, err
		}
	} else if !r.cfg.Limiter.TryAcquire() {
		closeRequestBody(req)
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
