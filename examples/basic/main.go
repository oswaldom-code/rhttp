package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func main() {
	// Create a client with middleware chain:
	// Timeout -> CircuitBreaker -> Retry
	client := rhttp.New(
		rhttp.WithMiddleware(
			rhttp.Timeout(10*time.Second),
			rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
				FailureThreshold: 5,
				ResetTimeout:     30 * time.Second,
			}),
			rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts: 3,
				Backoff:     rhttp.ExponentialBackoff(100*time.Millisecond, 5*time.Second),
			}),
		),
	)

	// Example 1: Simple GET request
	fmt.Println("=== Example 1: Simple GET ===")
	simpleGet(client)

	// Example 2: GET with query parameters
	fmt.Println("\n=== Example 2: GET with Query Params ===")
	getWithQueryParams(client)

	// Example 3: POST with JSON body
	fmt.Println("\n=== Example 3: POST with JSON ===")
	postJSON(client)

	// Example 4: Using path parameters
	fmt.Println("\n=== Example 4: Path Parameters ===")
	pathParams(client)

	// Example 5: Custom headers and timeout
	fmt.Println("\n=== Example 5: Custom Headers ===")
	customHeaders(client)
}

func simpleGet(client rhttp.Client) {
	resp, err := rhttp.R(client).
		Get("https://httpbin.org/get")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)
	printBody(resp.Body)
}

func getWithQueryParams(client rhttp.Client) {
	resp, err := rhttp.R(client).
		SetQueryParam("page", "1").
		SetQueryParam("limit", "10").
		SetQueryParams(map[string]string{
			"sort":  "created_at",
			"order": "desc",
		}).
		Get("https://httpbin.org/get")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)
	printBody(resp.Body)
}

func postJSON(client rhttp.Client) {
	payload := map[string]any{
		"name": "rhttp",
		"type": "library",
		"tags": []string{"http", "resilience", "go"},
	}

	resp, err := rhttp.R(client).
		SetBodyJSON(payload).
		Post("https://httpbin.org/post")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)
	printBody(resp.Body)
}

func pathParams(client rhttp.Client) {
	// Simulates: GET /users/123/posts/456
	resp, err := rhttp.R(client).
		SetPathParam("userId", "123").
		SetPathParam("postId", "456").
		Get("https://httpbin.org/anything/users/{userId}/posts/{postId}")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)
	printBody(resp.Body)
}

func customHeaders(client rhttp.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := rhttp.R(client).
		Context(ctx).
		SetHeader("X-Custom-Header", "custom-value").
		SetHeader("X-Request-ID", "req-12345").
		SetUserAgent("rhttp-example/1.0").
		SetAccept("application/json").
		Get("https://httpbin.org/headers")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)
	printBody(resp.Body)
}

func printBody(body io.Reader) {
	data, err := io.ReadAll(body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		return
	}

	var prettyJSON map[string]any
	if err := json.Unmarshal(data, &prettyJSON); err == nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(prettyJSON)
	} else {
		fmt.Println(string(data))
	}
}
