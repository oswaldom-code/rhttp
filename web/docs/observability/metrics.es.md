# Métricas

## Configuración

```go
rhttp.Metrics(rhttp.MetricsConfig{
    Recorder: myRecorder,
    PathNormalizer: func(path string) string {
        return normalize(path) // p. ej. /users/8f3a → /users/:id
    },
})
```

| Campo            | Significado                                                        |
| ---------------- | ------------------------------------------------------------------ |
| `Recorder`       | Sumidero de eventos de métrica (obligatorio)                       |
| `PathNormalizer` | Mapea rutas crudas a etiquetas de cardinalidad acotada (ver abajo) |

## La interfaz MetricsRecorder

```go
type MetricsRecorder interface {
    RecordRequest(event MetricEvent)
}
```

`MetricEvent` lleva método, host, ruta normalizada, código de estado y duración —
suficiente para construir métricas RED (rate, errors, duration) en Prometheus,
OpenTelemetry o lo que uses. `MetricsRecorderFunc` adapta una función corriente.

## Etiquetar los fallos

`MetricEvent.Error` es el error crudo. Pásalo por `Classify` para obtener una
etiqueta acotada y de baja cardinalidad:

```go
rhttp.MetricsRecorderFunc(func(e rhttp.MetricEvent) {
    kind := "none"
    if e.Error != nil {
        kind = rhttp.Classify(e.Error).Kind.String() // timeout, dns, circuit_open, …
    }
    counter.WithLabelValues(e.Method, kind).Inc()
})
```

En el orden recomendado, Metrics se sitúa **por encima** del circuit breaker y
del rate limiter, así que sus rechazos llegan al recolector como errores
ordinarios. Se clasifican como `circuit_open` y `rate_limited`: el cliente
protegiéndose a sí mismo, que es un hecho distinto de una dependencia que falló,
y merece una etiqueta distinta.

## Cardinalidad de las etiquetas

Rutas crudas como `/users/8f3a/orders/2941` crearían una serie temporal por cada
identificador y harían estallar tu backend de métricas. Para eso existe
`PathNormalizer`: si es nil, `MetricEvent.Path` se emite **vacío** (seguro por
defecto); proporciona un normalizador que colapse los segmentos variables para
emitir una plantilla acotada como `/users/:id/orders/:id`.
