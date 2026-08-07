# Timeouts

## Middleware

```go
rhttp.Timeout(5 * time.Second)
```

Applies a deadline to every request that passes through it. Semantics:

- If the request's context already carries a **shorter** deadline, it is
  respected and the middleware steps aside.
- The timeout covers the whole exchange **including reading the body**: the
  context is canceled when you `Close()` the response body, not before. Reading a
  large body past the deadline fails with `context.DeadlineExceeded`.
- A non-positive duration cannot bound anything, so the middleware falls back to
  a pass-through. Set [`OnInvalidConfig`](../observability/diagnostics.md) to
  learn about it at startup instead of during the incident.

## Per-request timeout

The builder can tighten the deadline for a single call:

```go
resp, err := client.R().
    Context(ctx).
    SetTimeout(800 * time.Millisecond).
    Get("https://api.example.com/health")
```

## Interaction with Retry

In the recommended order (`Timeout → Retry`) the timeout is a **total budget**
for all attempts and their backoff waits. That is usually what you want, but on
its own it leaves each attempt unbounded: a dependency that became slow lets the
first attempt consume the whole budget, and the retries never happen.

Bound the attempt with [`RetryConfig.AttemptTimeout`](retry.md#bounding-each-attempt),
not by placing `Timeout` beneath `Retry`:

```go
rhttp.Timeout(5*time.Second),            // total budget
rhttp.Retry(rhttp.RetryConfig{
    MaxAttempts:    3,
    AttemptTimeout: 2 * time.Second,     // per attempt
}),
```

Both bounds compose: the per-attempt context derives from the caller's, so it can
only shorten the operation. The middleware-order variant achieves the same thing
but expresses the dependency nowhere, and reverts silently if the chain is
reordered.
