# Configuración inválida

Un middleware al que se le da una configuración que no puede aplicar no falla en
la construcción: devuelve el `RoundTripper` siguiente sin tocarlo. `Timeout(0)` no
puede acotar nada, `RateLimit{Limiter: nil}` no tiene qué consultar,
`NewTokenBucket(0, …)` entraría en espera activa o bloquearía para siempre.
Degradar a un paso directo es la respuesta segura en los tres casos.

El problema no es la degradación, sino que antes era silenciosa. Un cliente al que
le falta una protección se ve exactamente igual que uno bien configurado, hasta
el día en que esa protección hacía falta.

## OnInvalidConfig

```go
func init() {
    rhttp.OnInvalidConfig = func(component, reason string) {
        log.Printf("rhttp: %s está inerte: %s", component, reason)
    }
}
```

El hook convierte una protección ausente en una señal de arranque, en vez de en
un hallazgo durante el incidente. Es nil por defecto, así que la degradación
sigue tan callada como en versiones anteriores hasta que decides activarlo.

Asígnalo **una sola vez durante el arranque, antes de construir cualquier cliente
o limitador**. Es una variable de paquete corriente: mutarla mientras otra
goroutine construye middleware es una carrera de datos.

## Qué se reporta y qué calla

| Componente    | Se reporta cuando                        |
| ------------- | ---------------------------------------- |
| `Timeout`     | La duración no es positiva               |
| `RateLimit`   | `Limiter` es nil                         |
| `TokenBucket` | `rate <= 0` o `burst < 1`                |

Solo reportan aquí los componentes cuya ausencia pierde una protección. Un
`MetricsConfig.Recorder` o un `LoggingConfig.Logger` nil significan
"observabilidad no configurada" — un valor por defecto legítimo que no pierde
nada, así que callan por diseño.

## Fallar en lugar de degradar

Para el rate limiter, donde los números suelen venir de configuración que podría
estar mal, hay una alternativa más estricta que devuelve el caso inválido como
error en vez de un bucket que no limita:

```go
limiter, err := rhttp.NewTokenBucketE(cfg.Rate, cfg.Burst)
if err != nil {
    return err // errors.Is(err, rhttp.ErrInvalidRateLimit)
}
```

Consulta [Rate limiting](../resiliency/rate-limiting.md#cuando-los-numeros-son-invalidos).
