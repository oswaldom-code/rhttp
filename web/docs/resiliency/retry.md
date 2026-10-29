# Retry & backoff

## Configuration

```go
rhttp.Retry(rhttp.RetryConfig{
    MaxAttempts: 3,
    Backoff: rhttp.WithRetryAfter(
        rhttp.ExponentialBackoff(100*time.Millisecond, 2*time.Second),
    ),
})
```

| Field             | Meaning                                                                 |
| ----------------- | ----------------------------------------------------------------------- |
| `MaxAttempts`     | Maximum attempts, including the first one                               |
| `AttemptTimeout`  | Deadline for each individual attempt (default: unbounded — see below)   |
| `Backoff`         | `BackoffFunc` deciding the wait before each retry (default exponential) |
| `IsRetryable`     | Custom predicate; default logic below                                   |
| `RetryAllMethods` | Retry non-idempotent methods too (default: idempotent only)             |

## What gets retried by default

- Errors classified as retryable: timeouts, connection failures, DNS errors.
- Status codes `429`, `502`, `503`, `504`. A plain `500` is **not** retried.
- Only idempotent methods, unless `RetryAllMethods` is set.
- Only requests whose body can be replayed (`GetBody` present or no body).

Context cancellation always wins: a canceled request is never retried, including
during a backoff wait.

## Bounding each attempt

A deadline on the caller's context bounds the **operation**, not each attempt.
Against a dependency that became slow rather than one that fails fast, the first
attempt consumes the entire budget and `MaxAttempts: 3` puts exactly one request
on the wire — while the logs and metrics report a plain timeout, which looks
identical to a retry sequence that legitimately ran out of time.

`AttemptTimeout` bounds the attempt itself:

```go
rhttp.Retry(rhttp.RetryConfig{
    MaxAttempts:    3,
    AttemptTimeout: 2 * time.Second, // each attempt; the context still caps the whole
})
```

The per-attempt context derives from the caller's, so it can only shorten the
operation, never extend it. Zero — the default — keeps the previous behavior.

!!! tip "Prefer it over middleware order"

    Composing `Timeout` *beneath* `Retry` achieves the same thing, but that is a
    dependency no type expresses: reorder the chain and the guarantee silently
    disappears. `AttemptTimeout` states it where it is read.

## Backoff strategies

All strategies are allocation-free and respect a maximum duration:

```go
rhttp.ConstantBackoff(500 * time.Millisecond)
rhttp.LinearBackoff(base, max)
rhttp.ExponentialBackoff(base, max)
rhttp.FibonacciBackoff(base, max)
rhttp.ExponentialBackoffFullJitter(base, max)
rhttp.ExponentialBackoffEqualJitter(base, max)
rhttp.DecorrelatedJitterBackoff(base, max)
```

Compose them with decorators:

```go
rhttp.WithJitter(backoff, 0.2)   // ±20% jitter
rhttp.WithMin(backoff, min)
rhttp.WithMax(backoff, max)
rhttp.WithRetryAfter(backoff)    // honor Retry-After on 429/503
```

`WithRetryAfter` parses both delay-seconds and HTTP-date forms and waits for
`max(backoff, server hint)`.

## BackoffFunc

```go
type BackoffFunc func(attempt int, resp *http.Response) time.Duration
```

The function receives the response that triggered the retry (`nil` if the attempt
produced none), which is what enables server-driven strategies like
`WithRetryAfter`.
