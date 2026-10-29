// Command retry validates the Retry middleware against httpbin, a public test
// service whose /status endpoint fails on purpose.
//
// Scenarios:
//   - a flaky endpoint (503 most of the time) that eventually succeeds
//   - a dead endpoint (always 503) that exhausts every attempt
//   - a 500, which DefaultIsRetryable treats as permanent and does not retry
//   - a POST, which is not retried because it is not idempotent
//
// Every request leaving the process is logged, so the attempts are visible.
//
//	go run ./examples/retry
//
// Point HTTPBIN_URL at a local instance to avoid depending on httpbin.org:
//
//	docker run --rm -p 8080:8080 mccutchen/go-httpbin
//	HTTPBIN_URL=http://localhost:8080 go run ./examples/retry
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func baseURL() string {
	if u := os.Getenv("HTTPBIN_URL"); u != "" {
		return u
	}
	return "https://httpbin.org"
}

// attemptLogger sits inside Retry, so it sees each attempt rather than the
// final outcome of the operation.
func attemptLogger(next http.RoundTripper) http.RoundTripper {
	return rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		start := time.Now()
		resp, err := next.RoundTrip(req)
		if err != nil {
			log.Printf("  attempt %s %s -> error: %v", req.Method, req.URL.Path, err)
			return resp, err
		}
		log.Printf("  attempt %s %s -> %d (%s)", req.Method, req.URL.Path, resp.StatusCode, time.Since(start).Round(time.Millisecond))
		return resp, err
	})
}

func newClient() *rhttp.Client {
	return rhttp.New(
		rhttp.WithMiddleware(
			rhttp.Timeout(30*time.Second),
			rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts:    5,
				AttemptTimeout: 5 * time.Second,
				Backoff:        rhttp.ExponentialBackoff(200*time.Millisecond, 2*time.Second),
			}),
			attemptLogger,
		),
	)
}

func run(ctx context.Context, client *rhttp.Client, title, method, url string) {
	fmt.Printf("\n=== %s ===\n", title)
	start := time.Now()

	resp, err := client.R().Context(ctx).Execute(method, url)
	if err != nil {
		fmt.Printf("result: error after %s: %v\n", time.Since(start).Round(time.Millisecond), err)
		return
	}
	// Retry returns the last response when attempts run out; the caller still
	// owns its body.
	resp.Body.Close()
	fmt.Printf("result: %d after %s\n", resp.StatusCode, time.Since(start).Round(time.Millisecond))
}

func main() {
	client := newClient()
	ctx := context.Background()
	base := baseURL()

	// httpbin picks one of the listed codes at random using the given weights.
	run(ctx, client, "flaky: 70% 503, 30% 200 -> retried until it succeeds",
		http.MethodGet, base+"/status/503:0.7,200:0.3")

	run(ctx, client, "dead: always 503 -> retried until MaxAttempts",
		http.MethodGet, base+"/status/503")

	run(ctx, client, "500 -> not retryable by default, single attempt",
		http.MethodGet, base+"/status/500")

	run(ctx, client, "POST to a 503 -> not idempotent, single attempt",
		http.MethodPost, base+"/status/503")
}
