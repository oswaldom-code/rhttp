# HTTP client comparison report

Generated: 2026-08-05 17:18 CEST

## Environment

| | |
|---|---|
| CPU | 12th Gen Intel(R) Core(TM) i7-1255U (12 threads) |
| OS/arch | linux/amd64 |
| Go | go1.24.1 |
| Samples per benchmark | 5 (min reported as typical cost) |

## Tool versions

- github.com/oswaldom-code/rhttp (local, via replace)
- github.com/go-resty/resty/v2 v2.17.2
- github.com/hashicorp/go-retryablehttp v0.7.8
- github.com/gojek/heimdall/v7 v7.0.3

## Methodology

All clients are configured equivalently: 5s timeout, 3 total attempts, exponential backoff 100ms-2s. Every client fully consumes and closes the response body.

- **Overhead**: a no-op transport returns 200 OK without touching the network, isolating client/middleware cost per request.
- **E2E**: a local httptest.Server returns ~1 KB of JSON over loopback, measuring total request cost including a real HTTP round trip.

Caveats: net/http does not retry (it is the floor, not a symmetric competitor); Heimdall runs without its Hystrix circuit breaker (retry only, for feature symmetry); Resty buffers the full response body by design; loopback amplifies relative overhead — against a real network (0.5-500 ms) these differences are negligible.

## Results: wrapper overhead (no network)

| Client | ns/op (min) | ns/op (mean) | B/op | allocs/op | vs best |
|---|---:|---:|---:|---:|---:|
| rhttp (Timeout+Retry) | 784 | 827 | 1275 | 10 | 1.00x |
| rhttp (Timeout+Retry+CircuitBreaker) | 785 | 860 | 1275 | 10 | 1.00x |
| net/http (Timeout only, no retry) | 1672 | 1726 | 1594 | 26 | 2.13x |
| go-retryablehttp | 1719 | 1810 | 1595 | 26 | 2.19x |
| Heimdall (retry) | 2579 | 2694 | 2221 | 32 | 3.29x |
| Resty (retry) | 5803 | 6056 | 4885 | 48 | 7.40x |

## Results: end-to-end (loopback, ~1 KB JSON)

| Client | ns/op (min) | ns/op (mean) | B/op | allocs/op | vs best |
|---|---:|---:|---:|---:|---:|
| go-retryablehttp | 60326 | 62835 | 6347 | 74 | 1.00x |
| net/http (Timeout only, no retry) | 61578 | 65103 | 6573 | 75 | 1.02x |
| Heimdall (retry) | 64415 | 66765 | 6973 | 80 | 1.07x |
| rhttp (Timeout+Retry+CircuitBreaker) | 64939 | 68159 | 7034 | 74 | 1.08x |
| rhttp (Timeout+Retry) | 66714 | 69344 | 7023 | 74 | 1.11x |
| Resty (retry) | 73722 | 78923 | 10979 | 96 | 1.22x |

## Reproduce

```bash
cd benchmarks
go run ./report -count 5
```
