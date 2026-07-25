// Package rhttp provides a production-grade HTTP client for Go with built-in
// resiliency patterns. It wraps the standard net/http package with middleware support
// for timeouts, retries, circuit breakers, rate limiting, logging, and metrics.
//
// # Quick Start
//
// Create a client with default settings:
//
//	client := rhttp.New()
//	resp, err := client.Do(ctx, req)
//
// Create a client with middleware:
//
//	client := rhttp.New(
//		rhttp.WithMiddleware(
//			rhttp.Timeout(5*time.Second),
//			rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3}),
//			rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
//				FailureThreshold: 5,
//				ResetTimeout:     30*time.Second,
//			}),
//		),
//	)
//
// # Middleware
//
// Middleware wraps http.RoundTripper to add cross-cutting concerns. The recommended
// order from outermost to innermost is:
//
//	Logging -> Metrics -> Timeout -> RateLimit -> Retry -> CircuitBreaker
//
// Available middleware:
//   - [Timeout]: Enforces request timeouts
//   - [Retry]: Retries failed requests with configurable backoff
//   - [CircuitBreaker]: Prevents cascading failures
//   - [RateLimit]: Controls request rate with token bucket algorithm
//   - [Logging]: Logs request/response details
//   - [Metrics]: Records request metrics
//
// # Fluent API
//
// For a more ergonomic API, use the RequestBuilder:
//
//	resp, err := client.R().
//		SetHeader("Authorization", "Bearer token").
//		SetQueryParam("page", "1").
//		SetBodyJSON(payload).
//		Post("https://api.example.com/users")
//
// # Backoff Strategies
//
// Multiple backoff strategies are available for retry configuration:
//   - [ConstantBackoff]: Fixed delay between retries
//   - [LinearBackoff]: Linearly increasing delay
//   - [ExponentialBackoff]: Exponentially increasing delay with jitter
//   - [FibonacciBackoff]: Fibonacci sequence based delay
//   - [DecorrelatedJitterBackoff]: AWS-recommended jitter algorithm
//   - [ExponentialBackoffFullJitter]: Full jitter for thundering herd prevention
//   - [ExponentialBackoffEqualJitter]: Equal jitter variant
//
// Strategies compose with [WithJitter], [WithMin], [WithMax], and
// [WithRetryAfter], which honors the Retry-After header on 429/503 responses.
//
// # Error Classification
//
// Errors are automatically classified using [Classify] to help with retry decisions:
//
//	classified := rhttp.Classify(err)
//	if classified.Kind == rhttp.ErrKindTimeout {
//		// Handle timeout
//	}
//
// Helper functions like [IsTimeout], [IsConnection], and [IsRetryable] provide
// convenient error checking.
//
// # Thread Safety
//
// All types in this package are safe for concurrent use unless otherwise noted.
// The [Client] can be shared across goroutines, and middleware implementations
// are designed to be thread-safe. The exception is [RequestBuilder]: each
// builder is meant for a single request from a single goroutine.
//
// # Zero Dependencies
//
// This package has no external dependencies beyond the Go standard library,
// making it suitable for projects that require minimal dependency footprint.
package rhttp
