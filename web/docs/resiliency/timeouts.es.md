# Timeouts

## Middleware

```go
rhttp.Timeout(5 * time.Second)
```

Aplica un plazo a toda petición que lo atraviesa. Semántica:

- Si el contexto de la petición ya lleva un plazo **más corto**, se respeta y el
  middleware se aparta.
- El timeout cubre el intercambio completo **incluida la lectura del cuerpo**: el
  contexto se cancela cuando haces `Close()` del cuerpo de la respuesta, no
  antes. Leer un cuerpo grande pasado el plazo falla con
  `context.DeadlineExceeded`.
- Una duración no positiva no puede acotar nada, así que el middleware degrada a
  un paso directo. Configura
  [`OnInvalidConfig`](../observability/diagnostics.md) para enterarte al arrancar
  y no durante el incidente.

## Timeout por petición

El builder puede ajustar el plazo para una sola llamada:

```go
resp, err := client.R().
    Context(ctx).
    SetTimeout(800 * time.Millisecond).
    Get("https://api.example.com/health")
```

## Interacción con Retry

En el orden recomendado (`Timeout → Retry`) el timeout es un **presupuesto total**
para todos los intentos y sus esperas de backoff. Eso suele ser lo que quieres,
pero por sí solo deja cada intento sin acotar: una dependencia que se volvió
lenta deja que el primer intento consuma todo el presupuesto, y los reintentos
nunca ocurren.

Acota el intento con
[`RetryConfig.AttemptTimeout`](retry.md#acotar-cada-intento), no colocando
`Timeout` por debajo de `Retry`:

```go
rhttp.Timeout(5*time.Second),            // presupuesto total
rhttp.Retry(rhttp.RetryConfig{
    MaxAttempts:    3,
    AttemptTimeout: 2 * time.Second,     // por intento
}),
```

Ambas cotas se componen: el contexto por intento deriva del contexto del
llamante, así que solo puede acortar la operación. La variante basada en el orden
del middleware logra lo mismo, pero no expresa la dependencia en ninguna parte y
revierte en silencio si se reordena la cadena.
