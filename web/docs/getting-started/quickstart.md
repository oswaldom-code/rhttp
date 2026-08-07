# Quickstart

## Build a client

A `Client` is configured once with functional options and is safe for concurrent
use. Middleware runs in the order listed — first is outermost.

```go
client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.Timeout(5*time.Second),
        rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3}),
        rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
            FailureThreshold: 5,
            ResetTimeout:     30 * time.Second,
        }),
    ),
)
```

The recommended full order is:

```
Logging → Metrics → Timeout → RateLimit → Retry → CircuitBreaker
```

Retry sits outside the circuit breaker so every attempt consults the circuit — a
tripped breaker cuts the remaining attempts.

## Make requests

Use `client.Do(ctx, req)` with a plain `*http.Request`, or the fluent builder:

```go
resp, err := client.R().
    Context(ctx).
    SetHeader("X-Request-ID", id).
    SetQueryParam("page", "2").
    SetPathParam("id", "42").
    SetBodyJSON(payload).
    Post("https://api.example.com/users/{id}/orders")
```

The builder covers headers, auth (`SetAuthToken`, `SetBasicAuth`), query and path
params, per-request timeout (`SetTimeout`), and JSON/XML/form bodies. Bodies passed
as an `io.Reader` are buffered up to 10 MB so retries can replay them; larger
bodies stream once and are not retried.

## Decode responses

```go
var user User
if err := rhttp.DecodeJSON(resp, &user); err != nil {
    return err
}
```

`DecodeJSON` streams the decode, always drains and closes the body (keeping
connections reusable), and returns an error for status codes >= 300.
