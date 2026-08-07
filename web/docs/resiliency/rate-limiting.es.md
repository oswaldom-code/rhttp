# Rate limiting

## Configuración

```go
rhttp.RateLimit(rhttp.RateLimitConfig{
    Limiter:     rhttp.NewTokenBucket(100, 20), // 100 req/s, ráfaga de 20
    WaitOnLimit: true,
})
```

| Campo               | Significado                                                          |
| ------------------- | -------------------------------------------------------------------- |
| `Limiter`           | El `RateLimiter` a consultar (obligatorio)                           |
| `WaitOnLimit`       | Esperar por un token en vez de fallar rápido (por defecto: fallar)   |
| `RespectRetryAfter` | Honrar las cabeceras `Retry-After` de las respuestas                 |

Con `WaitOnLimit: false`, una petición que no encuentra token falla de inmediato
con `rhttp.ErrRateLimited`. Con `true`, espera — respetando la cancelación del
contexto. `Classify` reporta el rechazo como `ErrKindRateLimited`: fue la propia
cuota del cliente la que rechazó la petición, que nunca llegó a la red.

## Token bucket

`NewTokenBucket(rate, burst)` implementa un token bucket clásico: `burst` tokens
disponibles de golpe, rellenados de forma continua a `rate` tokens por segundo.
Adquirir un token cuesta ~50ns sin asignaciones, así que el limitador no añade
sobrecosto medible a la ruta de la petición.

## Cuando los números son inválidos

Un rate no positivo o una ráfaga menor que 1 no pueden producir un limitador. En
lugar de entrar en espera activa o bloquear para siempre, `NewTokenBucket`
degrada a un bucket que admite todo — seguro, pero indistinguible de uno bien
configurado hasta que llega la carga que debía moderar. Esa degradación se
reporta a través de [`OnInvalidConfig`](../observability/diagnostics.md).

Cuando los valores vienen de configuración que podría estar mal, usa
`NewTokenBucketE` y falla en el arranque:

```go
limiter, err := rhttp.NewTokenBucketE(cfg.Rate, cfg.Burst)
if err != nil {
    return err // errors.Is(err, rhttp.ErrInvalidRateLimit)
}
```

Un `RateLimitConfig` con `Limiter` nil es igualmente un paso directo, y se
reporta de la misma forma.

## Trae tu propio limitador

El middleware acepta cualquier implementación de:

```go
type RateLimiter interface {
    TryAcquire() bool
    WaitContext(ctx context.Context) error
}
```

Esta es la costura para adaptar `golang.org/x/time/rate` o un limitador
distribuido sin que rhttp adopte la dependencia.
