// Command circuitbreaker validates the CircuitBreaker middleware against
// httpbin's /status endpoint, which fails on purpose.
//
// The breaker walks the full state machine:
//
//	closed --(3 consecutive 503)--> open --(ResetTimeout)--> half-open --(probe ok)--> closed
//
// While open, calls fail with ErrCircuitOpen without touching the network; the
// network counter proves it. The breaker guards the whole client, not a single
// URL, so switching from /status/503 to /status/200 stands in for the
// dependency recovering.
//
//	go run ./examples/circuitbreaker
//
// Point HTTPBIN_URL at a local instance to avoid depending on httpbin.org:
//
//	docker run --rm -p 8080:8080 mccutchen/go-httpbin
//	HTTPBIN_URL=http://localhost:8080 go run ./examples/circuitbreaker
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/oswaldom-code/rhttp"
)

const resetTimeout = 2 * time.Second

func baseURL() string {
	if u := os.Getenv("HTTPBIN_URL"); u != "" {
		return u
	}
	return "https://httpbin.org"
}

// networkCounter sits inside the breaker: it only counts requests the breaker
// let through.
func networkCounter(n *atomic.Int32) rhttp.Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			n.Add(1)
			return next.RoundTrip(req)
		})
	}
}

func call(ctx context.Context, client *rhttp.Client, url string, sent *atomic.Int32) {
	before := sent.Load()
	start := time.Now()
	resp, err := client.R().Context(ctx).Get(url)
	elapsed := time.Since(start).Round(time.Millisecond)
	hitNetwork := sent.Load() > before

	switch {
	case errors.Is(err, rhttp.ErrCircuitOpen):
		fmt.Printf("  rejected by breaker in %s (network=%t, kind=%s)\n", elapsed, hitNetwork, rhttp.Classify(err).Kind)
	case err != nil:
		fmt.Printf("  error in %s (network=%t): %v\n", elapsed, hitNetwork, err)
	default:
		resp.Body.Close()
		fmt.Printf("  %d in %s (network=%t)\n", resp.StatusCode, elapsed, hitNetwork)
	}
}

func main() {
	var sent atomic.Int32

	client := rhttp.New(rhttp.WithMiddleware(
		rhttp.Timeout(10*time.Second),
		rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 3,
			ResetTimeout:     resetTimeout,
			OnStateChange: func(from, to rhttp.CircuitState) {
				log.Printf("  circuit %s -> %s", from, to)
			},
		}),
		networkCounter(&sent),
	))

	ctx := context.Background()
	base := baseURL()

	fmt.Println("=== dependency down: 6 calls to /status/503 ===")
	for i := 0; i < 6; i++ {
		call(ctx, client, base+"/status/503", &sent)
	}

	fmt.Printf("\n=== waiting %s (ResetTimeout) ===\n", resetTimeout)
	time.Sleep(resetTimeout + 200*time.Millisecond)

	fmt.Println("\n=== dependency recovered: probe /status/200 ===")
	call(ctx, client, base+"/status/200", &sent)
	call(ctx, client, base+"/status/200", &sent)

	fmt.Printf("\nrequests sent to the network: %d of 8\n", sent.Load())
}
