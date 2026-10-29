# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| 0.2.x   | yes       |
| < 0.2   | no        |

## Reporting a vulnerability

Use GitHub private vulnerability reporting:
**Security → Report a vulnerability** on this repository. Do not open a public
issue for an unfixed vulnerability.

You will get an acknowledgement within 7 days and a fix or mitigation plan
within 30 days for confirmed reports.

## Scope

rhttp is a client library layered on `net/http`. It does not parse HTTP,
terminate TLS, or accept connections. The stdlib owns the protocol; rhttp owns
retry, timeout, circuit breaking, rate limiting and observability around it.

What rhttp guarantees:

- Context deadlines and cancellation are honored on every attempt.
- Retries never replay non-idempotent methods unless explicitly enabled.
- Response bodies from failed attempts are drained and closed before retrying.
- No external dependencies: the supply chain is the Go toolchain alone.

What rhttp does not guarantee:

- TLS configuration, proxy trust and redirect policy come from the
  `http.Transport` you provide.
- Request smuggling, header injection and similar protocol attacks are out of
  scope; report those to the Go project.

## Automated checks

Every push and pull request runs the Security workflow:

| Tool        | Purpose                                   |
|-------------|-------------------------------------------|
| gosec       | Static analysis for insecure Go patterns  |
| govulncheck | Known CVEs reachable from this code       |
| Semgrep     | `p/golang` ruleset                        |
| Trivy       | Secrets, misconfiguration, known vulns    |
