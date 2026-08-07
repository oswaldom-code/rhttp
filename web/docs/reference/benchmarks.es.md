# Benchmarks

Tres niveles, cada uno responde a una pregunta distinta. Todas las cifras están
medidas en linux/amd64 con el toolchain de Go 1.24.1, media de 5 ejecuciones.
rhttp en sí requiere **Go 1.21 o superior** — es lo que declara `go.mod` y contra
lo que corre CI (1.21, 1.22, 1.23); el toolchain del benchmark es simplemente el
que produjo estos números.

## Comparativa — sobrecosto del cliente

Timeout, retry y circuit breaker contra un transport simulado, de modo que solo
se mide el sobrecosto del cliente. La cadena se mantiene deliberadamente en lo
que las demás bibliotecas pueden expresar, y todos los clientes reciben la misma
configuración: timeout de 5s, 3 intentos, backoff exponencial de 100ms a 2s.

| Cliente           | tiempo/op medio | relativo | allocs/op |
| ----------------- | --------------: | -------: | --------: |
| **rhttp**         |     **~0.86µs** |    1.00× |        10 |
| net/http + retry  |         ~1.73µs |    2.01× |        26 |
| retryablehttp     |         ~1.81µs |    2.10× |        26 |
| heimdall          |         ~2.69µs |    3.13× |        32 |
| resty             |         ~6.06µs |    7.04× |        48 |

## Extremo a extremo — servidor en loopback

Contra un servidor HTTP real en loopback, rhttp con la cadena completa corre a
1.05–1.07× del suelo que marca `net/http` pelado (74 allocs/op). En este nivel
los resultados caen dentro de un ±10% de ruido entre ejecuciones — trata las
diferencias pequeñas de posición como empates.

## Micro — presupuestos por componente

| Componente                | Coste          | Asignaciones |
| ------------------------- | -------------- | -----------: |
| Estrategias de backoff    | 1.6–11.6 ns/op |            0 |
| Adquisición de TokenBucket| ~50 ns/op      |            0 |
| Clasificación de errores  | ~8 ns/op       |            0 |

Más allá de la comparativa de tres middleware de arriba, la cadena completa de
seis (que añade rate limit, logging y metrics) cuesta 12 allocs/op frente a las 4
de un cliente pelado — el presupuesto que vigila
`BenchmarkMiddlewareOverhead_AllMiddleware`. `AttemptTimeout` y `OnStateChange`
no añaden ninguna asignación mientras no se configuren.

## Advertencias

- Las cifras con transport simulado aíslan el sobrecosto del cliente; sobre una
  red real, la latencia las eclipsa a todas. El punto es que la resiliencia de
  rhttp es efectivamente gratis.
- Cada biblioteca se configuró de forma tan equivalente como permite su API; la
  paridad exacta de funcionalidades es imposible (p. ej. no todas soportan
  circuit breaking de forma nativa).
- Los números cambian con el hardware y la versión de Go. Reprodúcelos tú mismo:

```sh
git clone https://github.com/oswaldom-code/rhttp
cd rhttp/benchmarks && make report
```
