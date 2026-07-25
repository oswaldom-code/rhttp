# HTTP client comparison report

Generated: 2026-07-25 13:04 CEST

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
| rhttp (Timeout+Retry+CircuitBreaker) | 1012 | 1058 | 1468 | 12 | 1.00x |
| rhttp (Timeout+Retry) | 1019 | 1099 | 1468 | 12 | 1.01x |
| net/http (Timeout only, no retry) | 1784 | 1854 | 1594 | 26 | 1.76x |
| go-retryablehttp | 1909 | 1965 | 1595 | 26 | 1.89x |
| Heimdall (retry) | 2797 | 2969 | 2221 | 32 | 2.76x |
| Resty (retry) | 6486 | 6787 | 4885 | 48 | 6.41x |

## Results: end-to-end (loopback, ~1 KB JSON)

| Client | ns/op (min) | ns/op (mean) | B/op | allocs/op | vs best |
|---|---:|---:|---:|---:|---:|
| go-retryablehttp | 59892 | 63859 | 6357 | 74 | 1.00x |
| Heimdall (retry) | 61990 | 67502 | 6999 | 80 | 1.04x |
| net/http (Timeout only, no retry) | 62209 | 67362 | 6584 | 75 | 1.04x |
| rhttp (Timeout+Retry) | 63382 | 66040 | 7224 | 76 | 1.06x |
| rhttp (Timeout+Retry+CircuitBreaker) | 66125 | 67722 | 7185 | 76 | 1.10x |
| Resty (retry) | 72794 | 78349 | 10916 | 96 | 1.22x |

## Reproduce

```bash
cd benchmarks
go run ./report -count 5
```
