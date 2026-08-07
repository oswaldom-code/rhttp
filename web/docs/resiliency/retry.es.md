# Reintentos y backoff

## Configuración

```go
rhttp.Retry(rhttp.RetryConfig{
    MaxAttempts: 3,
    Backoff: rhttp.WithRetryAfter(
        rhttp.ExponentialBackoff(100*time.Millisecond, 2*time.Second),
    ),
})
```

| Campo             | Significado                                                                  |
| ----------------- | ---------------------------------------------------------------------------- |
| `MaxAttempts`     | Número máximo de intentos, incluido el primero                               |
| `AttemptTimeout`  | Plazo para cada intento individual (por defecto: sin acotar — ver abajo)     |
| `Backoff`         | `BackoffFunc` que decide la espera previa a cada reintento (por defecto exponencial) |
| `IsRetryable`     | Predicado propio; la lógica por defecto está más abajo                       |
| `RetryAllMethods` | Reintentar también métodos no idempotentes (por defecto: solo idempotentes)  |

## Qué se reintenta por defecto

- Errores clasificados como reintentables: timeouts, fallos de conexión y errores
  de DNS transitorios.
- Códigos de estado `429`, `502`, `503` y `504`. Un `500` a secas **no** se
  reintenta.
- Solo métodos idempotentes, salvo que se active `RetryAllMethods`.
- Solo peticiones cuyo cuerpo puede reproducirse (`GetBody` presente o sin cuerpo).

La cancelación del contexto siempre gana: una petición cancelada nunca se
reintenta, ni siquiera durante una espera de backoff.

## Acotar cada intento

Un plazo en el contexto del llamante acota la **operación**, no cada intento.
Frente a una dependencia que se volvió lenta —en vez de una que falla rápido— el
primer intento consume todo el presupuesto y `MaxAttempts: 3` pone exactamente
una petición en el cable, mientras los logs y las métricas reportan un timeout
corriente, indistinguible de una secuencia de reintentos que legítimamente se
quedó sin tiempo.

`AttemptTimeout` acota el intento en sí:

```go
rhttp.Retry(rhttp.RetryConfig{
    MaxAttempts:    3,
    AttemptTimeout: 2 * time.Second, // cada intento; el contexto sigue acotando el total
})
```

El contexto por intento deriva del contexto del llamante, así que solo puede
acortar la operación, nunca extenderla. Cero —el valor por defecto— mantiene el
comportamiento anterior.

!!! tip "Prefiérelo al orden de la cadena"

    Componer `Timeout` *por debajo* de `Retry` consigue lo mismo, pero esa es una
    dependencia que ningún tipo expresa: si alguien reordena la cadena, la
    garantía desaparece en silencio. `AttemptTimeout` la declara donde se lee.

## Estrategias de backoff

Todas las estrategias son libres de asignaciones y respetan una duración máxima:

```go
rhttp.ConstantBackoff(500 * time.Millisecond)
rhttp.LinearBackoff(base, max)
rhttp.ExponentialBackoff(base, max)
rhttp.FibonacciBackoff(base, max)
rhttp.ExponentialBackoffFullJitter(base, max)
rhttp.ExponentialBackoffEqualJitter(base, max)
rhttp.DecorrelatedJitterBackoff(base, max)
```

Combínalas con decoradores:

```go
rhttp.WithJitter(backoff, 0.2)   // ±20% de jitter
rhttp.WithMin(backoff, min)
rhttp.WithMax(backoff, max)
rhttp.WithRetryAfter(backoff)    // honra Retry-After en 429/503
```

`WithRetryAfter` interpreta tanto la forma en segundos como la de fecha HTTP, y
espera `max(backoff, sugerencia del servidor)`.

## BackoffFunc

```go
type BackoffFunc func(attempt int, resp *http.Response) time.Duration
```

La función recibe la respuesta que disparó el reintento (`nil` si el intento no
produjo ninguna), que es justamente lo que habilita estrategias dirigidas por el
servidor como `WithRetryAfter`.
