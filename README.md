# rhttp

Production-grade HTTP client for Go with built-in resiliency patterns.

[![CI](https://github.com/oswaldom-code/rhttp/actions/workflows/ci.yml/badge.svg)](https://github.com/oswaldom-code/rhttp/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/oswaldom-code/rhttp/branch/main/graph/badge.svg)](https://codecov.io/gh/oswaldom-code/rhttp)
[![Go Report Card](https://goreportcard.com/badge/github.com/oswaldom-code/rhttp)](https://goreportcard.com/report/github.com/oswaldom-code/rhttp)
[![Go Reference](https://pkg.go.dev/badge/github.com/oswaldom-code/rhttp.svg)](https://pkg.go.dev/github.com/oswaldom-code/rhttp)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)

## Motivation

Después de implementar clientes HTTP con patrones de resiliencia en múltiples proyectos
de microservicios, identificé un patrón recurrente:

1. **La stdlib no es suficiente** - `net/http` es potente pero no incluye retry,
   circuit breaker ni rate limiting
2. **Las dependencias son un problema** - Librerías como Resty traen dependencias
   transitivas que complican auditorías de seguridad y aumentan el tamaño del binario
3. **Reinventar la rueda es costoso** - Cada equipo termina escribiendo su propio
   wrapper con bugs sutiles en manejo de contextos, timeouts y connection pooling

Esta librería resuelve ese problema: **resiliencia production-ready con cero dependencias**.

### Usage Modes

| Modo | Cuándo usarlo |
|------|---------------|
| `go get` | Proyectos que aceptan dependencias externas |
| Copiar a `pkg/rhttp` | Políticas estrictas de zero-deps, vendor everything |

El código está diseñado para funcionar en ambos escenarios sin modificaciones.

## Features

- **Zero dependencies** - Only Go standard library
- **Faster than net/http** - 35% faster than `http.Client` baseline
- **Middleware architecture** - Composable, testable, extensible
- **Fluent API** - Resty-style request builder
- **Resiliency patterns** - Retry, circuit breaker, rate limiting, timeout
- **Multiple backoff strategies** - Constant, linear, exponential, Fibonacci, jitter variants
- **Object pooling** - Reduced allocations via `sync.Pool`
- **100% test coverage** - 101 tests

## Installation

```bash
go get github.com/oswaldom-code/rhttp
```

Requires Go 1.21+

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/oswaldom-code/rhttp"
)

func main() {
    // Create client with middleware
    client := rhttp.New(
        rhttp.WithMiddleware(
            rhttp.Timeout(5*time.Second),
            rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3}),
            rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
                FailureThreshold: 5,
                ResetTimeout:     30*time.Second,
            }),
        ),
    )

    // Make request
    req, _ := http.NewRequest("GET", "https://api.example.com/users", nil)
    resp, err := client.Do(context.Background(), req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    fmt.Println("Status:", resp.StatusCode)
}
```

### Fluent API

```go
client := rhttp.New()

// GET request with query params
resp, err := rhttp.R(client).
    SetHeader("Authorization", "Bearer token").
    SetQueryParam("page", "1").
    SetQueryParam("limit", "10").
    Get("https://api.example.com/users")

// POST request with JSON body
resp, err := rhttp.R(client).
    SetAuthToken("my-token").
    SetBodyJSON(map[string]string{
        "name":  "John",
        "email": "john@example.com",
    }).
    Post("https://api.example.com/users")

// Path parameters
resp, err := rhttp.R(client).
    SetPathParam("org", "acme").
    SetPathParam("repo", "api").
    Get("https://api.github.com/repos/{org}/{repo}")
```

## Middleware

### Timeout

```go
client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.Timeout(5*time.Second),
    ),
)
```

Respects existing context deadlines - uses the shorter of the two.

### Retry

```go
client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.Retry(rhttp.RetryConfig{
            MaxAttempts:     3,
            Backoff:         rhttp.ExponentialBackoff(100*time.Millisecond, 10*time.Second),
            IsRetryable:     rhttp.DefaultIsRetryable, // 429, 502, 503, 504
            RetryAllMethods: false, // Only retry idempotent methods by default
        }),
    ),
)
```

**Built-in backoff strategies:**

| Strategy | Description |
|----------|-------------|
| `ConstantBackoff(d)` | Always wait `d` |
| `LinearBackoff(base, max)` | `base * (attempt + 1)` |
| `ExponentialBackoff(base, max)` | `base * 2^attempt` with ±20% jitter |
| `FibonacciBackoff(base, max)` | `base * fib(attempt)` |
| `ExponentialBackoffFullJitter(base, max)` | `random(0, base * 2^attempt)` |
| `ExponentialBackoffEqualJitter(base, max)` | `base * 2^attempt / 2 + random(0, half)` |
| `DecorrelatedJitterBackoff(base, max)` | AWS-style decorrelated jitter |

Composable with `WithJitter()`, `WithMin()`, `WithMax()`.

### Circuit Breaker

```go
client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
            FailureThreshold: 5,           // Open after 5 consecutive failures
            ResetTimeout:     30*time.Second, // Try half-open after 30s
            IsFailure:        rhttp.DefaultIsFailure, // Errors + 5xx
        }),
    ),
)
```

State machine: `Closed → Open → Half-Open → Closed/Open`

Returns `rhttp.ErrCircuitOpen` when circuit is open.

### Rate Limiting

```go
// Token bucket: 100 requests/second, burst of 10
limiter := rhttp.NewTokenBucket(100, 10)

client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.RateLimit(rhttp.RateLimitConfig{
            Limiter:           limiter,
            WaitOnLimit:       true,  // Block until token available
            RespectRetryAfter: true,  // Honor Retry-After header
        }),
    ),
)

// Per-host rate limiting
perHostLimiter := rhttp.NewPerHostRateLimiter(50, 5) // 50 req/s per host
```

### Logging

```go
client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.Logging(rhttp.LoggingConfig{
            Logger: rhttp.LoggerFunc(func(e rhttp.LogEntry) {
                log.Printf("%s %s %d %v", e.Method, e.URL, e.StatusCode, e.Duration)
            }),
            ShouldLog: func(req *http.Request, resp *http.Response, err error) bool {
                return err != nil || resp.StatusCode >= 500 // Only log errors
            },
        }),
    ),
)
```

### Metrics

```go
client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.Metrics(rhttp.MetricsConfig{
            Recorder: rhttp.MetricsRecorderFunc(func(e rhttp.MetricEvent) {
                // Send to Prometheus, StatsD, etc.
                myCounter.WithLabels(e.Method, e.Host, e.StatusCode).Inc()
                myHistogram.Observe(e.Duration.Seconds())
            }),
        }),
    ),
)
```

`MetricEvent` fields: `Method`, `Host`, `Path`, `StatusCode`, `Duration`, `BytesSent`, `BytesReceived`, `Error`, `Success`

## Error Classification

```go
resp, err := client.Do(ctx, req)
if err != nil {
    classified := rhttp.Classify(err)

    switch classified.Kind {
    case rhttp.ErrKindTimeout:
        // Request timed out
    case rhttp.ErrKindCancelled:
        // Context was cancelled
    case rhttp.ErrKindConnection:
        // Connection refused, reset, etc.
    case rhttp.ErrKindDNS:
        // DNS resolution failed
    case rhttp.ErrKindTLS:
        // Certificate error
    case rhttp.ErrKindTemporary:
        // Temporary error, may resolve on retry
    }

    // Or use helpers
    if rhttp.IsRetryable(err) {
        // Safe to retry (timeout, connection, DNS, temporary)
    }
}
```

## Middleware Order

Middleware executes in the order specified:

```go
client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.Logging(...),        // 1. Log request start
        rhttp.Metrics(...),        // 2. Start timing
        rhttp.Timeout(...),        // 3. Apply timeout
        rhttp.RateLimit(...),      // 4. Check rate limit
        rhttp.CircuitBreaker(...), // 5. Check circuit
        rhttp.Retry(...),          // 6. Retry on failure
    ),
)
```

Recommended order: `Logging → Metrics → Timeout → RateLimit → CircuitBreaker → Retry`

## Custom Transport

```go
// Use custom transport
client := rhttp.New(
    rhttp.WithTransport(&http.Transport{
        MaxIdleConns:        200,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     90*time.Second,
    }),
)

// Or use optimized default
transport := rhttp.DefaultTransport() // HTTP/2 enabled, optimized pool
```

## Object Pooling

Reduce allocations with buffer pooling:

```go
// Get a buffer from the pool
buf := rhttp.GetBuffer()
defer rhttp.PutBuffer(buf)

buf.WriteString("request body")
```

## Benchmarks

```
goos: linux
goarch: amd64
cpu: Intel Core i7-1255U

BenchmarkClient_Baseline-12               235 ns/op    656 B/op    4 allocs/op
BenchmarkStdHttpClient_Baseline-12        317 ns/op    600 B/op    7 allocs/op  (+35%)
BenchmarkClient_WithRetry-12              265 ns/op    656 B/op    4 allocs/op
BenchmarkClient_WithCircuitBreaker-12     271 ns/op    656 B/op    4 allocs/op
BenchmarkClient_AllMiddleware-12         1143 ns/op   1472 B/op   12 allocs/op
BenchmarkTokenBucket_TryAcquire-12         52 ns/op      0 B/op    0 allocs/op
BenchmarkBackoff_Exponential-12             7 ns/op      0 B/op    0 allocs/op
```

**Key results:**
- 35% faster than `net/http` client baseline
- All middleware stack: ~1μs overhead (negligible vs network latency)
- Rate limiter: 52ns per check, zero allocations
- Backoff strategies: <10ns, zero allocations

## Design Principles

1. **No global state** - Each client is independent
2. **Context-first** - All operations respect context cancellation
3. **Fail fast** - Explicit errors, no silent failures
4. **Composable** - Mix and match middleware
5. **Testable** - All components are mockable
6. **Zero dependencies** - Only Go standard library

## API Reference

See [pkg.go.dev](https://pkg.go.dev/github.com/oswaldom-code/rhttp) for full API documentation.

## Development

### Prerequisites

```bash
# Install development tools
make install-tools
```

### Available Commands

```bash
make help          # Show all available commands
make test          # Run unit tests
make test-race     # Run tests with race detector
make test-coverage # Generate coverage report
make bench         # Run benchmarks
make lint          # Run golangci-lint
make fmt           # Format code
make vet           # Run go vet
make check         # Run all checks (fmt, vet, lint, test)
make docs          # Serve documentation locally
make clean         # Clean build artifacts
```

### CI Pipeline

The project uses GitHub Actions for CI with:

- Tests on Go 1.21, 1.22, and 1.23
- Race detector enabled
- golangci-lint for code quality
- Coverage reporting
- Benchmark tracking on PRs

## Contributing

Contributions are welcome! Please ensure:

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Run checks: `make check`
4. Commit changes: `git commit -m 'Add my feature'`
5. Push: `git push origin feature/my-feature`
6. Open a Pull Request

All PRs must pass CI checks before merging.

## Roadmap

> **Status:** Phase 1 complete. Phase 2 is the current focus.

### Phase 1: Foundation (Completed)

- [x] **Middleware architecture** - Composable, chained `http.RoundTripper`
- [x] **Functional options** - Configuration via `WithXxx()`
- [x] **Optimized transport** - HTTP/2, tuned connection pooling and timeouts
- [x] **Timeout middleware** - Context-aware, respects shorter deadlines
- [x] **Retry middleware** - Idempotency-safe with body replay
- [x] **Backoff strategies** - Constant, linear, exponential, Fibonacci, jitter variants
- [x] **Circuit breaker** - Closed/Open/Half-Open state machine
- [x] **Rate limiting** - Token bucket + per-host limiter
- [x] **Logging middleware** - Pluggable `Logger` interface
- [x] **Metrics middleware** - Pluggable `MetricsRecorder` interface
- [x] **Error classification** - Timeout, connection, DNS, TLS, temporary
- [x] **Fluent API** - Resty-style `RequestBuilder`
- [x] **Object pooling** - Reduced allocations via `sync.Pool`
- [x] **Zero dependencies** - Only Go standard library

### Phase 2: Advanced Resiliency

- [ ] **Circuit breaker per endpoint** - Separate circuit state for each host/path
- [ ] **Sliding window statistics** - Time-based failure rate calculation
- [ ] **Bulkhead pattern** - Resource isolation per service
- [ ] **Retry budget** - Limit retries per time window
- [ ] **Hedged requests** - Send duplicate request if first is slow
- [ ] **Adaptive timeout** - Adjust timeout based on latency percentiles

### Phase 3: Observability

- [ ] **OpenTelemetry integration** - Native tracing and metrics
- [ ] **slog compatibility** - Structured logging (Go 1.21+)
- [ ] **Prometheus metrics** - Out-of-the-box histograms and counters
- [ ] **Distributed tracing** - Automatic trace context propagation
- [ ] **Health check endpoints** - Readiness/liveness probes

### Phase 4: Developer Experience

- [ ] **Auto marshaling** - JSON, XML, Protocol Buffers, MessagePack
- [ ] **OAuth2 support** - Automatic token refresh
- [ ] **Debug mode** - Request/response dump, curl generation
- [ ] **Response validation** - JSON Schema, status assertions
- [ ] **Multipart uploads** - With progress callbacks

### Phase 5: Advanced Features

- [ ] **Load balancing** - Round-robin, weighted, least connections
- [ ] **Service discovery** - DNS SRV, Kubernetes, Consul
- [ ] **Response caching** - RFC 7234 compliant, pluggable backends
- [ ] **Request coalescing** - Single-flight for duplicate requests
- [ ] **HTTP/3 support** - QUIC protocol (optional)
- [ ] **Connection warm-up** - Pre-establish connections

### Phase 6: Enterprise

- [ ] **mTLS support** - Mutual TLS authentication
- [ ] **Certificate pinning** - Enhanced security
- [ ] **Secrets management** - Vault integration
- [ ] **Configuration hot-reload** - Runtime tuning
- [ ] **Chaos engineering** - Fault injection for testing

---

Want to contribute? Check the issues labeled `good first issue` or `help wanted`.

## License

MIT License - see [LICENSE](LICENSE) file.
