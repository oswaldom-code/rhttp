# Invalid configuration

A middleware given configuration it cannot apply does not fail construction: it
returns the next `RoundTripper` unchanged. `Timeout(0)` cannot bound anything,
`RateLimit{Limiter: nil}` has nothing to consult, `NewTokenBucket(0, …)` would
busy-loop or block forever. Falling back to a pass-through is the safe answer to
all three.

The problem is not the fallback — it is that it used to be silent. A client
missing a protection looks exactly like a correctly configured one, right up to
the day the protection was needed.

## OnInvalidConfig

```go
func init() {
    rhttp.OnInvalidConfig = func(component, reason string) {
        log.Printf("rhttp: %s is inert: %s", component, reason)
    }
}
```

The hook turns a missing protection into a startup signal instead of an incident
finding. It is nil by default, so the fallback stays as quiet as it was in
earlier versions until you opt in.

Assign it **once during startup, before constructing any client or limiter**. It
is a plain package variable: mutating it while another goroutine builds
middleware is a data race.

## What reports, and what stays silent

| Component     | Reported when                             |
| ------------- | ----------------------------------------- |
| `Timeout`     | The duration is non-positive              |
| `RateLimit`   | `Limiter` is nil                          |
| `TokenBucket` | `rate <= 0` or `burst < 1`                |

Only components whose absence loses a protection report here. A nil
`MetricsConfig.Recorder` or `LoggingConfig.Logger` means "observability not
configured" — a legitimate default that loses nothing, so it stays silent by
design.

## Failing instead of degrading

For the rate limiter, where the numbers usually come from configuration that
could be wrong, there is a stricter alternative that returns the invalid case as
an error rather than a bucket that does not limit:

```go
limiter, err := rhttp.NewTokenBucketE(cfg.Rate, cfg.Burst)
if err != nil {
    return err // errors.Is(err, rhttp.ErrInvalidRateLimit)
}
```

See [Rate limiting](../resiliency/rate-limiting.md#when-the-numbers-are-invalid).
