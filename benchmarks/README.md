# Comparison benchmarks

Standalone benchmark suite comparing rhttp against popular Go HTTP clients
(net/http, Resty, go-retryablehttp, Heimdall) under equivalent configuration.

This is a **separate Go module** so the root rhttp module stays zero-dependency.
It is excluded automatically from root builds and tests.

## Usage

```bash
cd benchmarks

# Full run + Markdown report (writes REPORT.md)
make report

# Raw benchmark output only
make bench
```

`go run ./report` accepts `-count N` (samples per benchmark, default 5),
`-bench REGEX` and `-out FILE`.

## Scenarios

- **Overhead**: no-op transport, isolates client/middleware cost per request.
- **E2E**: local `httptest.Server` returning ~1 KB JSON over loopback.

All clients: 5s timeout, 3 total attempts, exponential backoff 100ms-2s, body
fully consumed and closed. See the Methodology section of the generated report
for fairness caveats.
