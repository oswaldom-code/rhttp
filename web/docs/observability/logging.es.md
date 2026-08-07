# Logging

## Configuración

```go
rhttp.Logging(rhttp.LoggingConfig{
    Logger: myLogger,
    ShouldLog: func(req *http.Request, resp *http.Response, err error) bool {
        return err != nil || resp.StatusCode >= 400 // solo errores
    },
})
```

| Campo       | Significado                                                   |
| ----------- | ------------------------------------------------------------- |
| `Logger`    | Destino de las entradas de log (obligatorio)                  |
| `ShouldLog` | Predicado de filtrado; si es nil, se registra toda petición   |

## La interfaz Logger

```go
type Logger interface {
    Log(entry LogEntry)
}
```

`LogEntry` lleva el método, la URL, el estado, la duración y el error del
intercambio. Cualquier función puede actuar como logger mediante `LoggerFunc`,
lo que reduce la adaptación de `log/slog` a unas pocas líneas:

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

Coloca Logging primero en la cadena para que observe el desenlace final del
intercambio completo —incluidos reintentos y cortes del circuit breaker— como
una sola entrada.
