// Command chucknorris demonstrates rhttp against api.chucknorris.io, a small
// public API that needs no authentication.
//
// It shows the pieces you actually wire in production: the recommended
// middleware order, a per-attempt bound on top of the total budget, circuit
// transitions published as they happen, and misconfiguration reported at
// startup.
//
//	go run ./examples/chucknorris
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oswaldom-code/rhttp"
)

const apiBase = "https://api.chucknorris.io/jokes"

// joke is one item of the API payload. Only the fields we use are declared;
// the decoder ignores the rest.
type joke struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Value string `json:"value"`
}

type searchResult struct {
	Total  int    `json:"total"`
	Result []joke `json:"result"`
}

// newClient builds the client once. A Client is safe for concurrent use, so in
// a real program this belongs in your composition root, not per request.
//
// Middleware runs in the order listed — the first one is the outermost. Retry
// sits outside the breaker so every attempt consults the circuit.
func newClient() *rhttp.Client {
	return rhttp.New(
		rhttp.WithMiddleware(
			// Outermost: sees the final outcome of the whole exchange,
			// retries included, as a single entry.
			rhttp.Logging(rhttp.LoggingConfig{
				Logger: rhttp.LoggerFunc(func(e rhttp.LogEntry) {
					log.Printf("%s %s -> %d in %s", e.Method, e.URL, e.StatusCode, e.Duration.Round(time.Millisecond))
				}),
			}),

			// Total budget for the operation: all attempts plus their backoff.
			rhttp.Timeout(10*time.Second),

			// Be a polite client against a free public API.
			rhttp.RateLimit(rhttp.RateLimitConfig{
				Limiter:           rhttp.NewTokenBucket(5, 5),
				WaitOnLimit:       true,
				RespectRetryAfter: true,
			}),

			// AttemptTimeout bounds each attempt. Without it, a dependency
			// that turns slow lets the first attempt eat the whole budget and
			// MaxAttempts never comes into play.
			rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts:    3,
				AttemptTimeout: 3 * time.Second,
				Backoff: rhttp.WithRetryAfter(
					rhttp.ExponentialBackoff(200*time.Millisecond, 2*time.Second),
				),
			}),

			// Innermost: stop hammering the host once it is clearly down.
			rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
				FailureThreshold: 5,
				ResetTimeout:     30 * time.Second,
				OnStateChange: func(from, to rhttp.CircuitState) {
					log.Printf("circuit %s -> %s", from, to)
				},
			}),
		),
	)
}

// categories fetches the list of joke categories: a plain JSON array.
func categories(ctx context.Context, client *rhttp.Client) ([]string, error) {
	resp, err := client.R().
		Context(ctx).
		SetAccept("application/json").
		Get(apiBase + "/categories")
	if err != nil {
		return nil, err
	}

	var out []string
	if err := rhttp.DecodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// randomJoke fetches one random joke, optionally restricted to a category.
func randomJoke(ctx context.Context, client *rhttp.Client, category string) (joke, error) {
	rb := client.R().
		Context(ctx).
		SetAccept("application/json")

	if category != "" {
		rb = rb.SetQueryParam("category", category)
	}

	resp, err := rb.Get(apiBase + "/random")
	if err != nil {
		return joke{}, err
	}

	var out joke
	if err := rhttp.DecodeJSON(resp, &out); err != nil {
		return joke{}, err
	}
	return out, nil
}

// search runs a full-text search and returns at most limit results.
func search(ctx context.Context, client *rhttp.Client, query string, limit int) (searchResult, error) {
	resp, err := client.R().
		Context(ctx).
		SetAccept("application/json").
		SetQueryParam("query", query).
		Get(apiBase + "/search")
	if err != nil {
		return searchResult{}, err
	}

	// DecodeJSON streams the decode, drains and closes the body so the
	// connection stays reusable, and turns any status >= 300 into an error.
	var out searchResult
	if err := rhttp.DecodeJSON(resp, &out); err != nil {
		return searchResult{}, err
	}

	if len(out.Result) > limit {
		out.Result = out.Result[:limit]
	}
	return out, nil
}

// run executes the demo. It returns instead of exiting so the deferred cancel
// runs before main decides the exit code.
func run(client *rhttp.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cats, err := categories(ctx, client)
	if err != nil {
		return fmt.Errorf("categories: %w", err)
	}
	fmt.Printf("%d categories: %v\n\n", len(cats), cats)

	j, err := randomJoke(ctx, client, "dev")
	if err != nil {
		return fmt.Errorf("random joke: %w", err)
	}
	fmt.Printf("random dev joke (%s):\n  %s\n\n", j.ID, j.Value)

	found, err := search(ctx, client, "compiler", 3)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	fmt.Printf("search \"compiler\": %d matches, first %d:\n", found.Total, len(found.Result))
	for _, r := range found.Result {
		fmt.Printf("  - %s\n", r.Value)
	}
	return nil
}

func main() {
	// A middleware that cannot apply its configuration falls back to a
	// pass-through. Report it at startup instead of discovering the missing
	// protection during an incident. Set it before building any client.
	rhttp.OnInvalidConfig = func(component, reason string) {
		log.Printf("rhttp: %s is inert: %s", component, reason)
	}

	if err := run(newClient()); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
