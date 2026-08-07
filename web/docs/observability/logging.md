# Logging

## Configuration

```go
rhttp.Logging(rhttp.LoggingConfig{
    Logger: myLogger,
    ShouldLog: func(req *http.Request, resp *http.Response, err error) bool {
        return err != nil || resp.StatusCode >= 400 // errors only
    },
})
```

| Field       | Meaning                                            |
| ----------- | -------------------------------------------------- |
| `Logger`    | Destination for log entries (required)             |
| `ShouldLog` | Filter predicate; if nil, every request is logged  |

## The Logger interface

```go
type Logger interface {
    Log(entry LogEntry)
}
```

`LogEntry` carries the method, URL, status, duration and error of the exchange.
Any function can be a logger via `LoggerFunc`, which makes adapting `log/slog`
a few lines:

```go
logger := rhttp.LoggerFunc(func(e rhttp.LogEntry) {
    slog.Info("http",
        "method", e.Method,
        "url", e.URL,
        "status", e.StatusCode,
        "duration", e.Duration,
        "err", e.Error,
    )
})
```

Place Logging first in the chain so it observes the final outcome of the whole
exchange — including retries and circuit-breaker short-circuits — as a single
entry.
