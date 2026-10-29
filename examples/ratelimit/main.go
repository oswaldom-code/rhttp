// Command ratelimit validates client-side rate limiting and the handling of a
// server-side 429, using httpbin as the remote.
//
// Scenarios:
//   - WaitOnLimit=true: a burst of concurrent calls is spread out to the rate
//   - WaitOnLimit=false: calls over the quota fail fast with ErrRateLimited,
//     without touching the network
//   - the server answers 429 on purpose: Retry backs off and tries again
//
// Run it with:
//
//	go run ./examples/ratelimit
//
// Point HTTPBIN_URL at a local instance to avoid depending on httpbin.org:
//
//	docker run --rm -p 8080:8080 mccutchen/go-httpbin
//	HTTPBIN_URL=http://localhost:8080 go run ./examples/ratelimit
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/oswaldom-code/rhttp"
)

const calls = 6

func baseURL() string {
	if u := os.Getenv("HTTPBIN_URL"); u != "" {
		return u
	}
	return "https://httpbin.org"
}

// burst fires all calls at once through a client limited by cfg. It reports
// when each call left the limiter, which is what the bucket controls, and how
// it ended.
func burst(ctx context.Context, base string, cfg rhttp.RateLimitConfig) {
	start := time.Now()

	// released sits inside RateLimit: it runs once the limiter lets a call out.
	released := func(next http.RoundTripper) http.RoundTripper {
		return rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			fmt.Printf("  call %s: released at %s\n", req.Header.Get("X-Call"), time.Since(start).Round(10*time.Millisecond))
			return next.RoundTrip(req)
		})
	}

	client := rhttp.New(rhttp.WithMiddleware(
		rhttp.Timeout(15*time.Second),
		rhttp.RateLimit(cfg),
		released,
	))

	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := client.R().Context(ctx).SetHeader("X-Call", strconv.Itoa(i)).Get(base + "/get")
			switch {
			case errors.Is(err, rhttp.ErrRateLimited):
				fmt.Printf("  call %d: rejected locally (kind=%s)\n", i, rhttp.Classify(err).Kind)
			case err != nil:
				fmt.Printf("  call %d: error: %v\n", i, err)
			default:
				resp.Body.Close()
			}
		}(i)
	}
	wg.Wait()
}

func waitOnLimit(ctx context.Context, base string) {
	fmt.Printf("\n=== TokenBucket(2/s, burst 2), WaitOnLimit=true, %d concurrent calls ===\n", calls)
	burst(ctx, base, rhttp.RateLimitConfig{
		Limiter:     rhttp.NewTokenBucket(2, 2),
		WaitOnLimit: true,
	})
}

func failFast(ctx context.Context, base string) {
	fmt.Printf("\n=== TokenBucket(2/s, burst 2), WaitOnLimit=false, %d concurrent calls ===\n", calls)
	burst(ctx, base, rhttp.RateLimitConfig{
		Limiter: rhttp.NewTokenBucket(2, 2),
	})
}

// serverThrottles: the remote answers 429 most of the time. 429 is retryable by
// default; WithRetryAfter would honor a Retry-After header if the server sent
// one, and falls back to the base backoff otherwise.
func serverThrottles(ctx context.Context, base string) {
	fmt.Println("\n=== server answers 429 (70%) -> Retry with backoff ===")

	attempt := 0
	logAttempt := func(next http.RoundTripper) http.RoundTripper {
		return rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			attempt++
			resp, err := next.RoundTrip(req)
			if err == nil {
				fmt.Printf("  attempt %d -> %d (Retry-After=%q)\n", attempt, resp.StatusCode, resp.Header.Get("Retry-After"))
			}
			return resp, err
		})
	}

	client := rhttp.New(rhttp.WithMiddleware(
		rhttp.Timeout(20*time.Second),
		rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 6,
			Backoff:     rhttp.WithRetryAfter(rhttp.ExponentialBackoff(300*time.Millisecond, 3*time.Second)),
		}),
		logAttempt,
	))

	start := time.Now()
	resp, err := client.R().Context(ctx).Get(base + "/status/429:0.7,200:0.3")
	if err != nil {
		fmt.Printf("result: error: %v\n", err)
		return
	}
	resp.Body.Close()
	fmt.Printf("result: %d after %s\n", resp.StatusCode, time.Since(start).Round(time.Millisecond))
}

func main() {
	ctx := context.Background()
	base := baseURL()

	waitOnLimit(ctx, base)
	failFast(ctx, base)
	serverThrottles(ctx, base)
}
