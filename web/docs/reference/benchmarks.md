# Benchmarks

Three tiers, each answering a different question. All numbers measured on
linux/amd64 with the Go 1.24.1 toolchain, mean of 5 runs. rhttp itself requires
**Go 1.21 or later** — that is what `go.mod` declares and what CI tests against
(1.21, 1.22, 1.23); the benchmark toolchain is just what produced these figures.

## Comparative — client overhead

Timeout, retry and circuit breaker against a mock transport, so only client
overhead is measured. The chain is deliberately kept to what the other libraries
can express, and every client gets the same settings: 5s timeout, 3 attempts,
exponential backoff 100ms–2s.

| Client            | mean time/op | relative | allocs/op |
| ----------------- | -----------: | -------: | --------: |
| **rhttp**         |  **~0.86µs** |    1.00× |        10 |
| net/http + retry  |      ~1.73µs |    2.01× |        26 |
| retryablehttp     |      ~1.81µs |    2.10× |        26 |
| heimdall          |      ~2.69µs |    3.13× |        32 |
| resty             |      ~6.06µs |    7.04× |        48 |

## End-to-end — loopback server

Against a real HTTP server on loopback, rhttp with the full chain runs at
1.05–1.07× of the bare `net/http` floor (74 allocs/op). At this tier results sit
within ±10% run-to-run noise — treat small rank differences as ties.

## Micro — component budgets

| Component            | Cost           | Allocations |
| -------------------- | -------------- | ----------: |
| Backoff strategies   | 1.6–11.6 ns/op |           0 |
| TokenBucket acquire  | ~50 ns/op      |           0 |
| Error classification | ~8 ns/op       |           0 |

Beyond the three-middleware comparison above, the full six-middleware chain
(adding rate limit, logging and metrics) costs 12 allocs/op against the 4 of a
bare client — the budget `BenchmarkMiddlewareOverhead_AllMiddleware` guards.
`AttemptTimeout` and `OnStateChange` add no allocation while unset.

## Caveats

- Mock-transport numbers isolate client overhead; over a real network, latency
  dwarfs all of them. The point is that rhttp's resiliency is effectively free.
- Each library was configured as equivalently as its API allows; exact feature
  parity is impossible (e.g. not all support circuit breaking natively).
- Numbers move with hardware and Go versions. Reproduce them yourself:

```sh
git clone https://github.com/oswaldom-code/rhttp
cd rhttp/benchmarks && make report
```
