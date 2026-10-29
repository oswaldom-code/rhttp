// Command errorclass validates error classification against endpoints that
// fail on purpose, one per error kind:
//
//   - DNS NXDOMAIN: a name under .invalid, reserved to never resolve (RFC 6761)
//   - connection refused: a local port nobody listens on
//   - TLS: badssl.com, which serves broken certificates on purpose
//   - timeout: httpbin's /delay/{n} with a shorter client budget
//   - canceled: the caller cancels mid-flight
//
// For each one it prints the Kind returned by Classify and whether the kind is
// retryable, which is what drives DefaultIsRetryable.
//
//	go run ./examples/errorclass
//
// Point HTTPBIN_URL at a local instance to avoid depending on httpbin.org:
//
//	docker run --rm -p 8080:8080 mccutchen/go-httpbin
//	HTTPBIN_URL=http://localhost:8080 go run ./examples/errorclass
package main

import (
	"context"
	"fmt"
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

func classify(ctx context.Context, client *rhttp.Client, title, url string) {
	resp, err := client.R().Context(ctx).Get(url)
	if err == nil {
		resp.Body.Close()
		fmt.Printf("%-20s unexpected success: %d\n", title, resp.StatusCode)
		return
	}

	c := rhttp.Classify(err)
	fmt.Printf("%-20s kind=%-16s retryable=%-5t err=%v\n", title, c.Kind, c.Kind.IsRetryable(), err)
}

func main() {
	client := rhttp.New(rhttp.WithMiddleware(rhttp.Timeout(2 * time.Second)))
	ctx := context.Background()

	classify(ctx, client, "dns not found", "https://does-not-exist.invalid/")
	classify(ctx, client, "connection refused", "http://127.0.0.1:1/")
	classify(ctx, client, "tls expired", "https://expired.badssl.com/")
	classify(ctx, client, "tls self-signed", "https://self-signed.badssl.com/")
	classify(ctx, client, "tls wrong host", "https://wrong.host.badssl.com/")
	classify(ctx, client, "timeout", baseURL()+"/delay/5")

	canceled, cancel := context.WithCancel(ctx)
	time.AfterFunc(300*time.Millisecond, cancel)
	classify(canceled, client, "canceled", baseURL()+"/delay/5")
}
