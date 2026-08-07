<img src="../assets/banner.svg" alt="rhttp — cliente HTTP resiliente para Go, cero dependencias" class="rh-banner">

# Resiliencia integrada en cada petición { .rh-sr-only }

**rhttp** es un cliente HTTP para Go con reintentos, circuit breaking, rate limiting y
timeouts compuestos como una cadena de middleware transparente — con cero dependencias
externas y sobrecosto casi nulo.

[Empezar](getting-started/installation.md){ .md-button .md-button--primary }
[Ver en GitHub](https://github.com/oswaldom-code/rhttp){ .md-button }

## Instalación

```sh
go get github.com/oswaldom-code/rhttp
```

Requiere Go 1.21+. Sin dependencias transitivas: lo que auditas es lo que despliegas.

## Inicio rápido

Construyes el cliente una vez y describes las peticiones de forma fluida. Cada llamada
atraviesa la cadena resiliente completa.

=== "Cliente resiliente"

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

=== "Petición fluida"

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

## La cadena de middleware

Cada capacidad es una simple `func(http.RoundTripper) http.RoundTripper`. Componlas
en el orden recomendado — o trae las tuyas.

<div class="rh-chain">
  <span class="mw">Logging</span><span class="arr">→</span>
  <span class="mw">Metrics</span><span class="arr">→</span>
  <span class="mw">Timeout</span><span class="arr">→</span>
  <span class="mw">RateLimit</span><span class="arr">→</span>
  <span class="mw hot">Retry</span><span class="arr">→</span>
  <span class="mw state">CircuitBreaker</span><span class="arr">→</span>
  <span class="mw end">Transport</span>
</div>

Retry envuelve al breaker, de modo que cada intento consulta el circuito: un breaker
abierto corta los intentos restantes en lugar de castigar a un host que ya está
caído.

## Qué trae

<div class="grid cards" markdown>

-   **Reintentos inteligentes**

    Siete estrategias de backoff más decoradores, conscientes de `Retry-After`, con
    cuerpos reproducibles. [Saber más](resiliency/retry.md)

-   **Circuit breaker**

    Recuperación con sonda única y una máquina de estados con generaciones: una
    respuesta lenta y obsoleta nunca puede corromper la recuperación.
    [Saber más](resiliency/circuit-breaker.md)

-   **Rate limiting**

    Token bucket a ~50ns por adquisición, cero asignaciones, modos esperar-o-fallar.
    [Saber más](resiliency/rate-limiting.md)

-   **Builder fluido**

    Parámetros de ruta, query, autenticación, cuerpos JSON/XML/form — reintentables
    hasta 10 MB. [Saber más](getting-started/quickstart.md)

-   **Observabilidad**

    Hooks conectables de [logging](observability/logging.md) y
    [métricas](observability/metrics.md), transiciones del circuito en el momento en
    que ocurren y [configuración inválida reportada al
    arrancar](observability/diagnostics.md).

-   **Cero dependencias**

    Solo biblioteca estándar, más del 95% de cobertura de tests, limpio bajo el
    detector de carreras.

</div>

## Benchmarks

Timeout, retry y circuit breaker sobre el nivel de transport simulado, que aísla el
sobrecosto del cliente del ruido de red. Media de 5 ejecuciones, linux/amd64,
toolchain de Go 1.24.1.

| Cliente           | tiempo/op relativo | allocs/op |
| ----------------- | -----------------: | --------: |
| **rhttp**         |           **1.00×** |    **10** |
| net/http + retry  |              2.01× |        26 |
| retryablehttp     |              2.10× |        26 |
| heimdall          |              3.13× |        32 |
| resty             |              7.04× |        48 |

Metodología y advertencias en el [informe completo](reference/benchmarks.md).
