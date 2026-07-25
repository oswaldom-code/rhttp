# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-25

First public release.

### Added

- Middleware-based HTTP client (`New` returning `*Client`, `WithMiddleware`, `WithTransport`) built on `http.RoundTripper`, plus the exported `RoundTripperFunc` adapter that makes a custom middleware a one-liner.
- Resiliency middleware: `Timeout`, `Retry` with pluggable backoff, `CircuitBreaker`, and `RateLimit` (token bucket behind the `RateLimiter` interface: `TryAcquire` plus `WaitContext`).
- `SharedCircuitBreaker` (`NewCircuitBreaker`) for circuit state shared across multiple clients, with observable `State()` and `CircuitState.String()`.
- Observability middleware: `Logging` and `Metrics`. `MetricsConfig.PathNormalizer` bounds metrics label cardinality (raw path is omitted by default).
- Backoff strategies: constant, linear, exponential, Fibonacci, and their jitter variants — all zero-alloc and overflow-safe. `BackoffFunc` receives the response that triggered the retry, and the `WithRetryAfter` decorator honors the `Retry-After` header (delay-seconds or HTTP-date) on 429/503.
- Fluent request builder (`Client.R`) with JSON, XML, form and reader bodies, path parameters, and query parameters. Reader bodies up to 10 MB are buffered so retries can rewind them; larger bodies stream and are sent exactly once.
- `DecodeJSON` response helper: always drains and closes the body, fails on status >= 300.
- Error classification: `Classify`, `IsRetryable`, `IsTimeout`, `IsConnection`, and related helpers.
- Zero external dependencies; Go standard library only.

[0.1.0]: https://github.com/oswaldom-code/rhttp/releases/tag/v0.1.0
