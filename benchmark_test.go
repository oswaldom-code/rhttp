package rhttp_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
	"github.com/oswaldom-code/rhttp/internal"
)

// noopRoundTripper returns immediately with a 200 OK response.
// This isolates the benchmark to measure only client/middleware overhead.
var noopRoundTripper = internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Request:    req,
	}, nil
})

// noopLogger discards all log entries
var noopLogger = rhttp.LoggerFunc(func(rhttp.LogEntry) {})

// noopRecorder discards all metric events
var noopRecorder = rhttp.MetricsRecorderFunc(func(rhttp.MetricEvent) {})

func BenchmarkClient_Baseline(b *testing.B) {
	c := rhttp.New(rhttp.WithTransport(noopRoundTripper))
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.Do(ctx, req)
	}
}

func BenchmarkClient_WithTimeout(b *testing.B) {
	c := rhttp.New(
		rhttp.WithTransport(noopRoundTripper),
		rhttp.WithMiddleware(rhttp.Timeout(5*time.Second)),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.Do(ctx, req)
	}
}

func BenchmarkClient_WithRetry(b *testing.B) {
	c := rhttp.New(
		rhttp.WithTransport(noopRoundTripper),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
		})),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.Do(ctx, req)
	}
}

func BenchmarkClient_WithCircuitBreaker(b *testing.B) {
	c := rhttp.New(
		rhttp.WithTransport(noopRoundTripper),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 5,
			ResetTimeout:     30 * time.Second,
		})),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.Do(ctx, req)
	}
}

func BenchmarkClient_WithLogging(b *testing.B) {
	c := rhttp.New(
		rhttp.WithTransport(noopRoundTripper),
		rhttp.WithMiddleware(rhttp.Logging(rhttp.LoggingConfig{
			Logger: noopLogger,
		})),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.Do(ctx, req)
	}
}

func BenchmarkClient_WithMetrics(b *testing.B) {
	c := rhttp.New(
		rhttp.WithTransport(noopRoundTripper),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: noopRecorder,
		})),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.Do(ctx, req)
	}
}

func BenchmarkClient_AllMiddleware(b *testing.B) {
	c := rhttp.New(
		rhttp.WithTransport(noopRoundTripper),
		rhttp.WithMiddleware(
			rhttp.Timeout(5*time.Second),
			rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
				FailureThreshold: 5,
				ResetTimeout:     30 * time.Second,
			}),
			rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3}),
			rhttp.Logging(rhttp.LoggingConfig{Logger: noopLogger}),
			rhttp.Metrics(rhttp.MetricsConfig{Recorder: noopRecorder}),
		),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.Do(ctx, req)
	}
}

func BenchmarkClient_Parallel(b *testing.B) {
	c := rhttp.New(
		rhttp.WithTransport(noopRoundTripper),
		rhttp.WithMiddleware(
			rhttp.Timeout(5*time.Second),
			rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
				FailureThreshold: 5,
				ResetTimeout:     30 * time.Second,
			}),
			rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3}),
		),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Do(ctx, req)
		}
	})
}

// Comparison with standard http.Client
func BenchmarkStdHttpClient_Baseline(b *testing.B) {
	client := &http.Client{Transport: noopRoundTripper}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req = req.WithContext(ctx)
		_, _ = client.Do(req)
	}
}

func BenchmarkClassify_Error(b *testing.B) {
	err := context.DeadlineExceeded

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = rhttp.Classify(err)
	}
}
