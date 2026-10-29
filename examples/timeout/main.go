// Command timeout validates the Timeout middleware and Retry.AttemptTimeout
// against httpbin's /delay/{n} endpoint, which answers only after n seconds.
//
// Scenarios:
//   - a total budget shorter than the server delay: the call fails as a timeout
//   - a per-request SetTimeout that is tighter than the client budget
//   - a slow attempt cut by AttemptTimeout and retried within the total budget
//
// Run it with:
//
//	go run ./examples/timeout
//
// Point HTTPBIN_URL at a local instance to avoid depending on httpbin.org:
//
//	docker run --rm -p 8080:8080 mccutchen/go-httpbin
//	HTTPBIN_URL=http://localhost:8080 go run ./examples/timeout
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func baseURL() string {
	if u := os.Getenv("HTTPBIN_URL"); u != "" {
		return u
	}
	return "https://httpbin.org"
}

func report(start time.Time, resp *http.Response, err error) {
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		fmt.Printf("result: error after %s (timeout=%t, kind=%s): %v\n",
			elapsed, rhttp.IsTimeout(err), rhttp.Classify(err).Kind, err)
		return
	}
	resp.Body.Close()
	fmt.Printf("result: %d after %s\n", resp.StatusCode, elapsed)
}

// totalBudget: the server takes 5s, the client gives the whole operation 2s.
func totalBudget(ctx context.Context, base string) {
	fmt.Println("\n=== Timeout(2s) against /delay/5 ===")
	client := rhttp.New(rhttp.WithMiddleware(rhttp.Timeout(2 * time.Second)))

	start := time.Now()
	resp, err := client.R().Context(ctx).Get(base + "/delay/5")
	report(start, resp, err)
}

// perRequest: the client allows 10s, but this one call only tolerates 1s.
func perRequest(ctx context.Context, base string) {
	fmt.Println("\n=== Timeout(10s) + SetTimeout(1s) against /delay/3 ===")
	client := rhttp.New(rhttp.WithMiddleware(rhttp.Timeout(10 * time.Second)))

	start := time.Now()
	resp, err := client.R().Context(ctx).SetTimeout(time.Second).Get(base + "/delay/3")
	report(start, resp, err)
}

// attemptTimeout: without AttemptTimeout the first slow attempt would eat the
// whole 8s budget. With it, each attempt is cut at 1.5s and Retry tries again
// until MaxAttempts or the budget runs out.
func attemptTimeout(ctx context.Context, base string) {
	fmt.Println("\n=== Timeout(8s) + Retry{AttemptTimeout: 1.5s} against /delay/3 ===")

	var attempts atomic.Int32
	counter := func(next http.RoundTripper) http.RoundTripper {
		return rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			n := attempts.Add(1)
			start := time.Now()
			resp, err := next.RoundTrip(req)
			fmt.Printf("  attempt %d ended after %s (err=%v)\n", n, time.Since(start).Round(time.Millisecond), err)
			return resp, err
		})
	}

	client := rhttp.New(rhttp.WithMiddleware(
		rhttp.Timeout(8*time.Second),
		rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts:    3,
			AttemptTimeout: 1500 * time.Millisecond,
			Backoff:        rhttp.ConstantBackoff(200 * time.Millisecond),
		}),
		counter,
	))

	start := time.Now()
	resp, err := client.R().Context(ctx).Get(base + "/delay/3")
	report(start, resp, err)
}

func main() {
	ctx := context.Background()
	base := baseURL()

	totalBudget(ctx, base)
	perRequest(ctx, base)
	attemptTimeout(ctx, base)
}
