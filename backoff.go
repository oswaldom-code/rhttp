package rhttp

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// BackoffFunc returns the duration to wait before the nth retry attempt.
// attempt is 0-indexed (0 = first retry, 1 = second retry, etc.)
// resp is the response of the attempt that triggered the retry; it is nil when
// that attempt produced no response (e.g. a transport error). Pure strategies
// ignore it; decorators like [WithRetryAfter] use it.
type BackoffFunc func(attempt int, resp *http.Response) time.Duration

// ConstantBackoff returns a backoff function that always returns the same duration.
func ConstantBackoff(d time.Duration) BackoffFunc {
	return func(_ int, _ *http.Response) time.Duration {
		return d
	}
}

// LinearBackoff returns a backoff function with linear growth.
// The wait time is: base * (attempt + 1), capped at maxDuration.
func LinearBackoff(base, maxDuration time.Duration) BackoffFunc {
	return func(attempt int, _ *http.Response) time.Duration {
		backoff := base * time.Duration(attempt+1)
		if backoff > maxDuration {
			return maxDuration
		}
		return backoff
	}
}

// overflowsExp reports whether base * 2^attempt overflows int64.
func overflowsExp(base time.Duration, attempt int) bool {
	return attempt >= 63 || base > math.MaxInt64>>attempt
}

// ExponentialBackoff returns a backoff function with exponential growth and jitter.
// The wait time is: base * 2^attempt with ±20% jitter, capped at maxDuration.
// Attempts whose product no longer fits in int64 saturate at maxDuration.
func ExponentialBackoff(base, maxDuration time.Duration) BackoffFunc {
	return func(attempt int, _ *http.Response) time.Duration {
		if overflowsExp(base, attempt) {
			return maxDuration
		}
		backoff := base * (1 << attempt)
		backoff = min(backoff, maxDuration)
		// Add jitter: ±20% (not crypto, just randomization for backoff distribution)
		jitter := float64(backoff) * 0.2 * (rand.Float64()*2 - 1) //nolint:gosec
		return backoff + time.Duration(jitter)
	}
}

// FibonacciBackoff returns a backoff function based on the Fibonacci sequence.
// The wait time is: base * fib(attempt + 1), capped at maxDuration.
// Fibonacci: 1, 1, 2, 3, 5, 8, 13, 21, 34, 55...
func FibonacciBackoff(base, maxDuration time.Duration) BackoffFunc {
	return func(attempt int, _ *http.Response) time.Duration {
		fib := fibonacci(attempt + 1)
		backoff := base * time.Duration(fib)
		if backoff > maxDuration {
			return maxDuration
		}
		return backoff
	}
}

// fibonacci returns the nth Fibonacci number (1-indexed: 1,1,2,3,5,8...).
func fibonacci(n int) int {
	if n <= 2 {
		return 1
	}
	a, b := 1, 1
	for i := 3; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}

// DecorrelatedJitterBackoff returns a backoff with decorrelated jitter.
// This algorithm provides better distribution than exponential backoff with jitter.
// See: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
//
// The returned BackoffFunc carries shared state guarded by a mutex: it is
// race-free, but concurrent retry sequences feed the same last-backoff value
// and correlate their delays. Use one instance per sequence when that matters.
func DecorrelatedJitterBackoff(base, maxDuration time.Duration) BackoffFunc {
	var (
		mu          sync.Mutex
		lastBackoff time.Duration
	)
	return func(attempt int, _ *http.Response) time.Duration {
		mu.Lock()
		defer mu.Unlock()

		if attempt == 0 {
			lastBackoff = base
			return base
		}

		// Algorithm: sleep = min(cap, random_between(base, sleep * 3))
		minVal := float64(base)
		maxVal := float64(lastBackoff) * 3
		backoff := time.Duration(minVal + rand.Float64()*(maxVal-minVal)) //nolint:gosec

		backoff = min(backoff, maxDuration)
		lastBackoff = backoff
		return backoff
	}
}

// ExponentialBackoffFullJitter returns exponential backoff with full jitter.
// The wait time is: random(0, base * 2^attempt), capped at maxDuration.
// This provides the best spread for avoiding thundering herd.
func ExponentialBackoffFullJitter(base, maxDuration time.Duration) BackoffFunc {
	return func(attempt int, _ *http.Response) time.Duration {
		if overflowsExp(base, attempt) {
			return maxDuration
		}
		ceiling := base * (1 << attempt)
		ceiling = min(ceiling, maxDuration)
		return time.Duration(rand.Float64() * float64(ceiling)) //nolint:gosec
	}
}

// ExponentialBackoffEqualJitter returns exponential backoff with equal jitter.
// The wait time is: (base * 2^attempt)/2 + random(0, (base * 2^attempt)/2)
func ExponentialBackoffEqualJitter(base, maxDuration time.Duration) BackoffFunc {
	return func(attempt int, _ *http.Response) time.Duration {
		if overflowsExp(base, attempt) {
			return maxDuration
		}
		ceiling := base * (1 << attempt)
		ceiling = min(ceiling, maxDuration)
		half := ceiling / 2
		return half + time.Duration(rand.Float64()*float64(half)) //nolint:gosec
	}
}

// WithJitter wraps a backoff function and adds random jitter.
// jitterFraction should be between 0 and 1 (e.g., 0.2 for ±20% jitter).
func WithJitter(backoff BackoffFunc, jitterFraction float64) BackoffFunc {
	if jitterFraction <= 0 {
		return backoff
	}
	if jitterFraction > 1 {
		jitterFraction = 1
	}

	return func(attempt int, resp *http.Response) time.Duration {
		d := backoff(attempt, resp)
		jitter := float64(d) * jitterFraction * (rand.Float64()*2 - 1) //nolint:gosec
		result := d + time.Duration(jitter)
		if result < 0 {
			return 0
		}
		return result
	}
}

// WithMax wraps a backoff function and caps the maximum duration.
func WithMax(backoff BackoffFunc, maxDuration time.Duration) BackoffFunc {
	return func(attempt int, resp *http.Response) time.Duration {
		d := backoff(attempt, resp)
		if d > maxDuration {
			return maxDuration
		}
		return d
	}
}

// WithMin wraps a backoff function and ensures a minimum duration.
func WithMin(backoff BackoffFunc, minDuration time.Duration) BackoffFunc {
	return func(attempt int, resp *http.Response) time.Duration {
		d := backoff(attempt, resp)
		if d < minDuration {
			return minDuration
		}
		return d
	}
}

// parseRetryAfter extracts the wait hinted by a 429 or 503 response's
// Retry-After header, either as delay-seconds or as an HTTP-date.
func parseRetryAfter(resp *http.Response) (time.Duration, bool) {
	if resp == nil {
		return 0, false
	}
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable {
		return 0, false
	}
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := time.Until(t); d > 0 {
			return d, true
		}
	}
	return 0, false
}

// WithRetryAfter wraps a backoff function and honors the Retry-After header
// on 429 and 503 responses: the wait is max(base backoff, server hint).
// The wait can still be cut short by canceling the request context.
func WithRetryAfter(backoff BackoffFunc) BackoffFunc {
	return func(attempt int, resp *http.Response) time.Duration {
		d := backoff(attempt, resp)
		if hint, ok := parseRetryAfter(resp); ok && hint > d {
			return hint
		}
		return d
	}
}
