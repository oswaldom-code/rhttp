# Metrics

## Configuration

```go
rhttp.Metrics(rhttp.MetricsConfig{
    Recorder: myRecorder,
    PathNormalizer: func(path string) string {
        return normalize(path) // e.g. /users/8f3a → /users/:id
    },
})
```

| Field            | Meaning                                                       |
| ---------------- | ------------------------------------------------------------- |
| `Recorder`       | Sink for metric events (required)                             |
| `PathNormalizer` | Maps raw paths to bounded-cardinality labels (see below)      |

## The MetricsRecorder interface

```go
type MetricsRecorder interface {
    RecordRequest(event MetricEvent)
}
```

`MetricEvent` carries method, host, normalized path, status code and duration —
enough to build RED metrics (rate, errors, duration) in Prometheus, OpenTelemetry
or anything else. `MetricsRecorderFunc` adapts a plain function.

## Labelling failures

`MetricEvent.Error` is the raw error. Pass it through `Classify` to get a
bounded, low-cardinality label:

```go
rhttp.MetricsRecorderFunc(func(e rhttp.MetricEvent) {
    kind := "none"
    if e.Error != nil {
        kind = rhttp.Classify(e.Error).Kind.String() // timeout, dns, circuit_open, …
    }
    counter.WithLabelValues(e.Method, kind).Inc()
})
```

In the recommended order Metrics sits **above** the circuit breaker and the rate
limiter, so their refusals reach the recorder as ordinary errors. They classify
as `circuit_open` and `rate_limited` — the client protecting itself, which is a
different fact from a dependency that failed, and worth a different label.

## Label cardinality

Raw URL paths like `/users/8f3a/orders/2941` would create one time series per ID
and blow up your metrics backend. That is why `PathNormalizer` exists: if nil,
`MetricEvent.Path` is emitted **empty** (safe by default); provide a normalizer
that collapses variable segments to emit a bounded template such as
`/users/:id/orders/:id`.
