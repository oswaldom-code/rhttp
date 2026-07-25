package benchmarks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gojek/heimdall/v7/httpclient"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/oswaldom-code/rhttp"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

var noopTransport = rtFunc(func(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Request:    req,
	}, nil
})

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func newRhttpFull(rt http.RoundTripper) *rhttp.Client {
	return rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(
			rhttp.Timeout(5*time.Second),
			rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts: 3,
				Backoff:     rhttp.ExponentialBackoff(100*time.Millisecond, 2*time.Second),
			}),
			rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
				FailureThreshold: 5,
				ResetTimeout:     30 * time.Second,
			}),
		),
	)
}

func newRhttpRetryOnly(rt http.RoundTripper) *rhttp.Client {
	return rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(
			rhttp.Timeout(5*time.Second),
			rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts: 3,
				Backoff:     rhttp.ExponentialBackoff(100*time.Millisecond, 2*time.Second),
			}),
		),
	)
}

func newResty(rt http.RoundTripper) *resty.Client {
	return resty.New().
		SetTransport(rt).
		SetTimeout(5 * time.Second).
		SetRetryCount(2).
		SetRetryWaitTime(100 * time.Millisecond).
		SetRetryMaxWaitTime(2 * time.Second)
}

func newRetryable(rt http.RoundTripper) *retryablehttp.Client {
	c := retryablehttp.NewClient()
	c.HTTPClient = &http.Client{Transport: rt, Timeout: 5 * time.Second}
	c.RetryMax = 2
	c.RetryWaitMin = 100 * time.Millisecond
	c.RetryWaitMax = 2 * time.Second
	c.Logger = nil
	return c
}

func newHeimdall(rt http.RoundTripper) *httpclient.Client {
	return httpclient.NewClient(
		httpclient.WithHTTPClient(&http.Client{Transport: rt, Timeout: 5 * time.Second}),
		httpclient.WithRetryCount(2),
	)
}

func benchRhttp(b *testing.B, c *rhttp.Client, url string) {
	req, _ := http.NewRequest(http.MethodGet, url, http.NoBody)
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := c.Do(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		drain(resp)
	}
}

func benchNetHTTP(b *testing.B, c *http.Client, url string) {
	req, _ := http.NewRequest(http.MethodGet, url, http.NoBody)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := c.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		drain(resp)
	}
}

func benchResty(b *testing.B, c *resty.Client, url string) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := c.R().Get(url)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchRetryable(b *testing.B, c *retryablehttp.Client, url string) {
	req, _ := retryablehttp.NewRequest(http.MethodGet, url, nil)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := c.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		drain(resp)
	}
}

func benchHeimdall(b *testing.B, c *httpclient.Client, url string) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := c.Get(url, nil)
		if err != nil {
			b.Fatal(err)
		}
		drain(resp)
	}
}

func BenchmarkOverhead_NetHTTP_Bare(b *testing.B) {
	benchNetHTTP(b, &http.Client{Transport: noopTransport, Timeout: 5 * time.Second}, "http://example.com/users")
}

func BenchmarkOverhead_Rhttp_TimeoutRetry(b *testing.B) {
	benchRhttp(b, newRhttpRetryOnly(noopTransport), "http://example.com/users")
}

func BenchmarkOverhead_Rhttp_FullStack(b *testing.B) {
	benchRhttp(b, newRhttpFull(noopTransport), "http://example.com/users")
}

func BenchmarkOverhead_Resty_Retry(b *testing.B) {
	benchResty(b, newResty(noopTransport), "http://example.com/users")
}

func BenchmarkOverhead_Retryablehttp(b *testing.B) {
	benchRetryable(b, newRetryable(noopTransport), "http://example.com/users")
}

func BenchmarkOverhead_Heimdall_Retry(b *testing.B) {
	benchHeimdall(b, newHeimdall(noopTransport), "http://example.com/users")
}

var payload = []byte(`{"users":[{"id":1,"name":"Ada Lovelace","email":"ada@example.com","active":true},{"id":2,"name":"Grace Hopper","email":"grace@example.com","active":true},{"id":3,"name":"Alan Turing","email":"alan@example.com","active":false},{"id":4,"name":"Dennis Ritchie","email":"dennis@example.com","active":true},{"id":5,"name":"Ken Thompson","email":"ken@example.com","active":true},{"id":6,"name":"Rob Pike","email":"rob@example.com","active":true},{"id":7,"name":"Robert Griesemer","email":"robert@example.com","active":true},{"id":8,"name":"Russ Cox","email":"russ@example.com","active":true},{"id":9,"name":"Brad Fitzpatrick","email":"brad@example.com","active":false},{"id":10,"name":"Ian Lance Taylor","email":"ian@example.com","active":true}],"total":10,"page":1,"per_page":10,"has_more":false}`)

func newServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
}

func BenchmarkE2E_NetHTTP_Bare(b *testing.B) {
	srv := newServer()
	defer srv.Close()
	benchNetHTTP(b, &http.Client{Timeout: 5 * time.Second}, srv.URL)
}

func BenchmarkE2E_Rhttp_TimeoutRetry(b *testing.B) {
	srv := newServer()
	defer srv.Close()
	benchRhttp(b, newRhttpRetryOnly(http.DefaultTransport), srv.URL)
}

func BenchmarkE2E_Rhttp_FullStack(b *testing.B) {
	srv := newServer()
	defer srv.Close()
	benchRhttp(b, newRhttpFull(http.DefaultTransport), srv.URL)
}

func BenchmarkE2E_Resty_Retry(b *testing.B) {
	srv := newServer()
	defer srv.Close()
	benchResty(b, newResty(http.DefaultTransport), srv.URL)
}

func BenchmarkE2E_Retryablehttp(b *testing.B) {
	srv := newServer()
	defer srv.Close()
	benchRetryable(b, newRetryable(http.DefaultTransport), srv.URL)
}

func BenchmarkE2E_Heimdall_Retry(b *testing.B) {
	srv := newServer()
	defer srv.Close()
	benchHeimdall(b, newHeimdall(http.DefaultTransport), srv.URL)
}
