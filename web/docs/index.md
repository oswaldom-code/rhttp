<img src="assets/banner.svg" alt="rhttp — resilient HTTP client for Go, zero dependencies" class="rh-banner">

# Resiliency built into every request { .rh-sr-only }

**rhttp** is an HTTP client for Go with retries, circuit breaking, rate limiting and
timeouts composed as a transparent middleware chain — with zero external dependencies
and near-zero overhead.

[Get started](getting-started/installation.md){ .md-button .md-button--primary }
[View on GitHub](https://github.com/oswaldom-code/rhttp){ .md-button }

## Install

```sh
go get github.com/oswaldom-code/rhttp
```

Requires Go 1.21+. No transitive dependencies: what you audit is what you ship.

## Quickstart

Build a client once, describe requests fluently. Every call goes through the full
resilient chain.

=== "Resilient client"

    ```go
    client := rhttp.New(
        rhttp.WithMiddleware(
            rhttp.Timeout(5*time.Second),
            rhttp.Retry(rhttp.RetryConfig{
                MaxAttempts: 3,
                Backoff: rhttp.WithRetryAfter(
                    rhttp.ExponentialBackoff(100*time.Millisecond, 2*time.Second),
                ),
            }),
            rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
                FailureThreshold: 5,
            }),
        ),
    )
    ```

=== "Fluent request"

    ```go
    resp, err := client.R().
        Context(ctx).
        SetPathParam("id", "42").
        SetAccept("application/json").
        Get("https://api.example.com/users/{id}")
    if err != nil {
        return err
    }

    var user User
    if err := rhttp.DecodeJSON(resp, &user); err != nil {
        return err
    }
    ```

## The middleware chain

Every capability is a plain `func(http.RoundTripper) http.RoundTripper`. Compose them
in the recommended order — or bring your own.

<div class="rh-chain">
  <span class="mw">Logging</span><span class="arr">→</span>
  <span class="mw">Metrics</span><span class="arr">→</span>
  <span class="mw">Timeout</span><span class="arr">→</span>
  <span class="mw">RateLimit</span><span class="arr">→</span>
  <span class="mw hot">Retry</span><span class="arr">→</span>
  <span class="mw state">CircuitBreaker</span><span class="arr">→</span>
  <span class="mw end">Transport</span>
</div>

Retry wraps the breaker, so every attempt consults the circuit: a tripped breaker
short-circuits the remaining attempts instead of hammering a host that is already
down.

## What's in the box

<div class="grid cards" markdown>

-   **Smart retries**

    Seven backoff strategies plus decorators, `Retry-After` aware, replay-safe
    bodies. [Learn more](resiliency/retry.md)

-   **Circuit breaker**

    Single-probe recovery and a generation-gated state machine: slow stale
    responses can never corrupt recovery. [Learn more](resiliency/circuit-breaker.md)

-   **Rate limiting**

    Token bucket at ~50ns per acquire, zero allocations, wait-or-fail modes.
    [Learn more](resiliency/rate-limiting.md)

-   **Fluent builder**

    Path params, query, auth, JSON/XML/form bodies — retryable up to 10 MB.
    [Learn more](getting-started/quickstart.md)

-   **Observability**

    Pluggable [logging](observability/logging.md) and
    [metrics](observability/metrics.md) hooks, circuit transitions as they happen,
    and [misconfiguration reported at startup](observability/diagnostics.md).

-   **Zero dependencies**

    Standard library only, 95%+ test coverage, race-detector clean.

</div>

## Benchmarks

Timeout, retry and circuit breaker on the mock-transport tier, which isolates
client overhead from network noise. Mean of 5 runs, linux/amd64, Go 1.24.1
toolchain.

| Client            | time/op relative | allocs/op |
| ----------------- | ---------------: | --------: |
| **rhttp**         |        **1.00×** |    **10** |
| net/http + retry  |            2.01× |        26 |
| retryablehttp     |            2.10× |        26 |
| heimdall          |            3.13× |        32 |
| resty             |            7.04× |        48 |

Methodology and caveats in the [full report](reference/benchmarks.md).
