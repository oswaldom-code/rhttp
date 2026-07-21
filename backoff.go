package rhttp

import (
	"math/rand"
	"sync"
	"time"
)

// BackoffFunc returns the duration to wait before the nth retry attempt.
// attempt is 0-indexed (0 = first retry, 1 = second retry, etc.)
type BackoffFunc func(attempt int) time.Duration

// ConstantBackoff returns a backoff function that always returns the same duration.
func ConstantBackoff(d time.Duration) BackoffFunc {
	return func(_ int) time.Duration {
		return d
	}
}

// LinearBackoff returns a backoff function with linear growth.
// The wait time is: base * (attempt + 1), capped at maxDuration.
func LinearBackoff(base, maxDuration time.Duration) BackoffFunc {
	return func(attempt int) time.Duration {
		backoff := base * time.Duration(attempt+1)
		if backoff > maxDuration {
			return maxDuration
		}
		return backoff
	}
}

// ExponentialBackoff returns a backoff function with exponential growth and jitter.
// The wait time is: base * 2^attempt with ±20% jitter, capped at maxDuration.
func ExponentialBackoff(base, maxDuration time.Duration) BackoffFunc {
	return func(attempt int) time.Duration {
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
	return func(attempt int) time.Duration {
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
// Note: This function returns a stateful BackoffFunc that is safe for concurrent use.
func DecorrelatedJitterBackoff(base, maxDuration time.Duration) BackoffFunc {
	var (
		mu          sync.Mutex
		lastBackoff time.Duration
	)
	return func(attempt int) time.Duration {
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
	return func(attempt int) time.Duration {
		ceiling := base * (1 << attempt)
		ceiling = min(ceiling, maxDuration)
		return time.Duration(rand.Float64() * float64(ceiling)) //nolint:gosec
	}
}

// ExponentialBackoffEqualJitter returns exponential backoff with equal jitter.
// The wait time is: (base * 2^attempt)/2 + random(0, (base * 2^attempt)/2)
func ExponentialBackoffEqualJitter(base, maxDuration time.Duration) BackoffFunc {
	return func(attempt int) time.Duration {
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

	return func(attempt int) time.Duration {
		d := backoff(attempt)
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
	return func(attempt int) time.Duration {
		d := backoff(attempt)
		if d > maxDuration {
			return maxDuration
		}
		return d
	}
}

// WithMin wraps a backoff function and ensures a minimum duration.
func WithMin(backoff BackoffFunc, minDuration time.Duration) BackoffFunc {
	return func(attempt int) time.Duration {
		d := backoff(attempt)
		if d < minDuration {
			return minDuration
		}
		return d
	}
}
