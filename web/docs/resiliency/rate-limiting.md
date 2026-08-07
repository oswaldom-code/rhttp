# Rate limiting

## Configuration

```go
rhttp.RateLimit(rhttp.RateLimitConfig{
    Limiter:     rhttp.NewTokenBucket(100, 20), // 100 req/s, burst of 20
    WaitOnLimit: true,
})
```

| Field               | Meaning                                                          |
| ------------------- | ---------------------------------------------------------------- |
| `Limiter`           | The `RateLimiter` to consult (required)                          |
| `WaitOnLimit`       | Wait for a token instead of failing fast (default: fail fast)    |
| `RespectRetryAfter` | Honor `Retry-After` headers from responses                       |

With `WaitOnLimit: false`, a request that finds no token fails immediately with
`rhttp.ErrRateLimited`. With `true`, it waits — respecting context cancellation.
`Classify` reports the refusal as `ErrKindRateLimited`: the client's own quota
turned the request away, and it never reached the network.

## Token bucket

`NewTokenBucket(rate, burst)` implements a classic token bucket: `burst` tokens
available at once, refilled continuously at `rate` tokens per second. Acquiring a
token costs ~50ns with zero allocations, so the limiter adds no measurable
overhead to the request path.

## When the numbers are invalid

A non-positive rate or a burst below 1 cannot produce a limiter. Rather than
busy-loop or block forever, `NewTokenBucket` falls back to a bucket that admits
everything — safe, but indistinguishable from a correctly configured one until
the load it was meant to shape arrives. The fallback is reported through
[`OnInvalidConfig`](../observability/diagnostics.md).

When the values come from configuration that could be wrong, use
`NewTokenBucketE` and fail at startup instead:

```go
limiter, err := rhttp.NewTokenBucketE(cfg.Rate, cfg.Burst)
if err != nil {
    return err // errors.Is(err, rhttp.ErrInvalidRateLimit)
}
```

A `RateLimitConfig` with a nil `Limiter` is likewise a pass-through, and is
reported the same way.

## Bring your own limiter

The middleware accepts any implementation of:

```go
type RateLimiter interface {
    TryAcquire() bool
    WaitContext(ctx context.Context) error
}
```

This is the seam for adapting `golang.org/x/time/rate` or a distributed limiter
without rhttp taking the dependency.
