# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Four reports from `docs/issues` closed. Every change is additive and backward
compatible: no existing behavior changes unless a new field or hook is set.

### Added

- `ErrKindCircuitOpen` and `ErrKindRateLimited` error kinds. `Classify` used to report the package's own sentinels as `ErrKindUnknown`, so a metrics recorder wired above the circuit breaker — the arrangement the documentation suggests — labeled the breaker engaging as an unidentified failure, which is the opposite of what had happened. **No retry verdict changes:** both kinds are non-retryable, exactly as `ErrKindUnknown` was, and retrying either would defeat the protection that produced it. The kinds are appended last in the `ErrorKind` block, so the numeric values shipped in 0.1.0 and 0.2.0 are unchanged.
- `RetryConfig.AttemptTimeout` bounds each individual attempt. A deadline on the caller's context bounds the operation as a whole, so against a dependency that became slow rather than one that fails fast, the first attempt consumed the entire deadline and `MaxAttempts: 3` produced exactly one request on the wire — with every log and metric reporting a plain timeout. The only cure was composing `Timeout` beneath `Retry`, a dependency no type expressed and which silently reverted if the middleware order changed. Zero, the default, keeps the previous semantics.
- `CircuitBreakerConfig.OnStateChange` reports every state transition. At the default `SuccessThreshold` of 1 the half-open phase begins and ends inside a single `RoundTrip`, so no polling frequency can sample it: the transition that shows whether a dependency recovered on its own was unobservable by construction. The callback runs with the breaker's mutex released, on the goroutine of the request that caused the transition, so reading `State()` from inside it is safe.
- `CircuitBreakerWithState` returns the middleware together with the breaker it built. The plain `CircuitBreaker` form discards it, leaving the state of a circuit configured that way unreachable.
- `OnInvalidConfig`, a package-level hook called when a constructor receives configuration it cannot apply and falls back to a pass-through. `Timeout(0)`, `RateLimit{Limiter: nil}` and `NewTokenBucket(0, …)` used to lose a requested protection in complete silence — a client that looks identical to a correctly configured one until the day the protection was needed. Nil by default, which keeps the previous silence. `Metrics{Recorder: nil}` and `Logging{Logger: nil}` stay silent by design: the zero value there means "observability not configured", which is a legitimate default.
- `NewTokenBucketE`, `NewTokenBucket` with the invalid cases returned as an error wrapping the new `ErrInvalidRateLimit` sentinel, instead of degraded to a bucket that does not limit.

### Changed

- Doc comments for `Timeout`, `RateLimit` and `NewTokenBucket` now describe the no-op as a fallback rather than a project convention, and point at `OnInvalidConfig`. The previous wording read as a design principle, which is the part that surprised.
- `RetryConfig.MaxAttempts` documents that a context deadline bounds the operation, not each attempt, and points at `AttemptTimeout`.

### Performance

- `Classify` is unchanged on the common path: the two sentinels are matched by identity before the transport branches, and by `errors.Is` after them for the wrapped case. Classifying a transport failure stays at ~7.4 ns and zero allocations (measured against ~7.3 ns before the change); an unwrapped sentinel costs ~2.2 ns. Using only `errors.Is` measured 22 ns on the common path when placed first, and 535 ns with 8 allocations on the sentinel path when placed last, so both positions are used deliberately. `BenchmarkClassify_Sentinel` guards the identity check.
- The full middleware stack is unchanged at 12 allocations; the retry and circuit-breaker paths add no allocation when `AttemptTimeout` is zero and `OnStateChange` is nil.

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
