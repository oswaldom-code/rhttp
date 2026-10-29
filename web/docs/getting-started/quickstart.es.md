# Inicio rápido

## Construir un cliente

Un `Client` se configura una sola vez mediante opciones funcionales y es seguro
para uso concurrente. El middleware se ejecuta en el orden en que se lista: el
primero es el más externo.

```go
client := rhttp.New(
    rhttp.WithMiddleware(
        rhttp.Timeout(5*time.Second),
        rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3}),
        rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
            FailureThreshold: 5,
            ResetTimeout:     30 * time.Second,
        }),
    ),
)
```

El orden completo recomendado es:

```
Logging → Metrics → Timeout → RateLimit → Retry → CircuitBreaker
```

Retry queda por fuera del circuit breaker para que cada intento consulte el
circuito: un breaker abierto corta los intentos restantes.

## Hacer peticiones

Usa `client.Do(ctx, req)` con un `*http.Request` normal, o el builder fluido:

```go
resp, err := client.R().
    Context(ctx).
    SetHeader("X-Request-ID", id).
    SetQueryParam("page", "2").
    SetPathParam("id", "42").
    SetBodyJSON(payload).
    Post("https://api.example.com/users/{id}/orders")
```

El builder cubre cabeceras, autenticación (`SetAuthToken`, `SetBasicAuth`),
parámetros de query y de ruta, timeout por petición (`SetTimeout`) y cuerpos
JSON/XML/form. Los cuerpos que se pasan como `io.Reader` se bufferizan hasta
10 MB para que los reintentos puedan reproducirlos; los más grandes se
transmiten una sola vez y no se reintentan.

## Decodificar respuestas

```go
var user User
if err := rhttp.DecodeJSON(resp, &user); err != nil {
    return err
}
```

`DecodeJSON` decodifica en streaming, siempre drena y cierra el cuerpo —
manteniendo las conexiones reutilizables— y devuelve error para códigos de
estado >= 300.
