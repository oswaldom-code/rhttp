# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-08-05

### Added

- `ErrKindDNSNotFound` error kind and the `IsDNSNotFound` helper, for a name that does not exist (NXDOMAIN). The kind is appended last in the `ErrorKind` block, so the numeric values shipped in 0.1.0 are unchanged.

### Fixed

- `DefaultIsRetryable` no longer retries permanent DNS failures. `classifyError` now consults `net.DNSError.IsNotFound`: an NXDOMAIN classifies as the non-retryable `ErrKindDNSNotFound`, while a transient resolution failure stays `ErrKindDNS` and stays retryable. A misspelled or decommissioned hostname previously consumed the whole attempt budget plus the full backoff schedule on an outcome that could never succeed.

### Changed

- `IsDNS` now reports true for both `ErrKindDNS` and `ErrKindDNSNotFound`: an NXDOMAIN is still a DNS failure. Callers that need only the permanent case should use `IsDNSNotFound`.

### Performance

- The `Timeout` middleware attaches its context with `req.WithContext` instead of `req.Clone`. The deep copy was redundant — `Do` already clones the caller's request before the chain runs — and cost a duplicated struct, URL and header map on every request. The full middleware stack drops from 13 to 11 allocations and ~1589 to ~1304 B/op.

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

[0.2.0]: https://github.com/oswaldom-code/rhttp/releases/tag/v0.2.0
[0.1.0]: https://github.com/oswaldom-code/rhttp/releases/tag/v0.1.0
