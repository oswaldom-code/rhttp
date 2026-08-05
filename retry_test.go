package rhttp_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetry_SuccessAfterRetry(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     func(int, *http.Response) time.Duration { return time.Millisecond },
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_MaxAttemptsExhausted(t *testing.T) {
	var attempts int32
	expectedErr := syscall.ECONNREFUSED
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, expectedErr
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     func(int, *http.Response) time.Duration { return time.Millisecond },
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	if err != expectedErr {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_NonIdempotentMethodNotRetried(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		// A genuinely retryable error: proves the method guard is what stops the retry.
		return nil, syscall.ECONNREFUSED
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3})),
	)

	req, _ := http.NewRequest(http.MethodPost, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if attempts != 1 {
		t.Fatalf("expected 1 attempt for POST, got %d", attempts)
	}
}

func TestRetry_NonIdempotentMethodWithRetryAllMethods(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			return nil, syscall.ECONNREFUSED
		}
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts:     3,
			RetryAllMethods: true,
			Backoff:         func(int, *http.Response) time.Duration { return time.Millisecond },
		})),
	)

	body := bytes.NewReader([]byte("test"))
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", body)
	req.GetBody = func() (io.ReadCloser, error) {
		body.Seek(0, io.SeekStart)
		return io.NopCloser(body), nil
	}

	resp, err := c.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetry_ContextCancelledDuringBackoff(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, syscall.ECONNREFUSED
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     func(int, *http.Response) time.Duration { return 10 * time.Second },
		})),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(ctx, req)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt before context cancel, got %d", attempts)
	}
}

func TestRetry_RetryableStatusCodes(t *testing.T) {
	retryableCodes := []int{
		http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	for _, code := range retryableCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var attempts int32
			rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				n := atomic.AddInt32(&attempts, 1)
				if n < 2 {
					return &http.Response{
						StatusCode: code,
						Body:       io.NopCloser(strings.NewReader("")),
						Request:    req,
					}, nil
				}
				return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
			})

			c := rhttp.New(
				rhttp.WithTransport(rt),
				rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
					MaxAttempts: 3,
					Backoff:     func(int, *http.Response) time.Duration { return time.Millisecond },
				})),
			)

			req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
			resp, _ := c.Do(context.Background(), req)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected retry to succeed, got status %d", resp.StatusCode)
			}
			if attempts != 2 {
				t.Fatalf("expected 2 attempts, got %d", attempts)
			}
		})
	}
}

func TestRetry_LastAttemptBodyReadable(t *testing.T) {
	const payload = "final-503-body"
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader(payload)),
			Request:    req,
		}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     func(int, *http.Response) time.Duration { return time.Millisecond },
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading last-attempt body: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("expected body %q, got %q", payload, body)
	}
}

func TestRetry_NonRetryableStatusCode(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{MaxAttempts: 3})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, _ := c.Do(context.Background(), req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for non-retryable status, got %d", attempts)
	}
}

// nonReplayableReader is a reader that cannot be rewound.
type nonReplayableReader struct {
	r io.Reader
}

func (n *nonReplayableReader) Read(p []byte) (int, error) {
	return n.r.Read(p)
}

func TestRetry_NonReplayableBodyNotRetried(t *testing.T) {
	var attempts int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		// A genuinely retryable error: proves the body guard is what stops the retry.
		return nil, syscall.ECONNREFUSED
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     func(int, *http.Response) time.Duration { return time.Millisecond },
		})),
	)

	// Custom reader that http.NewRequest won't recognize - GetBody will be nil
	body := &nonReplayableReader{r: strings.NewReader("data")}
	req, _ := http.NewRequest(http.MethodPut, "http://example.com", body)
	// Explicitly clear GetBody to ensure it's not set
	req.GetBody = nil
	_, _ = c.Do(context.Background(), req)

	if attempts != 1 {
		t.Fatalf("expected 1 attempt for non-replayable body, got %d", attempts)
	}
}

type countingBody struct {
	io.Reader
	read int64
}

func (c *countingBody) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	c.read += int64(n)
	return n, err
}

func (c *countingBody) Close() error { return nil }

func TestRetry_DrainIsBounded(t *testing.T) {
	body := &countingBody{Reader: strings.NewReader(strings.Repeat("a", 8<<20))}
	first := true
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if first {
			first = false
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body, Request: req}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	wrapped := rhttp.Retry(rhttp.RetryConfig{
		MaxAttempts: 2,
		Backoff:     func(int, *http.Response) time.Duration { return 0 },
	})(rt)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = wrapped.RoundTrip(req)

	const maxDrain = 256 << 10
	if body.read > maxDrain {
		t.Fatalf("drain read %d bytes of an 8 MB body; max acceptable: %d", body.read, maxDrain)
	}
}

func TestRetry_RespectsErrorClassification(t *testing.T) {
	tlsErr := &url.Error{
		Op:  "Get",
		URL: "https://example.com",
		Err: &tls.CertificateVerificationError{},
	}

	cases := []struct {
		name     string
		err      error
		attempts int32
	}{
		{"tls_not_retryable", tlsErr, 1},
		{"canceled_not_retryable", context.Canceled, 1},
		{"connection_retryable", syscall.ECONNREFUSED, 3},
		{"dns_transient_retryable", &net.DNSError{Err: "server misbehaving", IsTemporary: true}, 3},
		{"dns_notfound_not_retryable", &net.DNSError{Err: "no such host", IsNotFound: true}, 1},
		{"timeout_retryable", context.DeadlineExceeded, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int32
			rt := rhttp.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
				atomic.AddInt32(&attempts, 1)
				return nil, tc.err
			})
			wrapped := rhttp.Retry(rhttp.RetryConfig{
				MaxAttempts: 3,
				Backoff:     func(int, *http.Response) time.Duration { return 0 },
			})(rt)

			req, _ := http.NewRequest(http.MethodGet, "https://example.com", http.NoBody)
			_, _ = wrapped.RoundTrip(req)

			if attempts != tc.attempts {
				t.Fatalf("%s: got %d attempts, want %d", tc.name, attempts, tc.attempts)
			}
		})
	}
}

func TestRetry_DoesNotMutateOriginalRequest(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody, Request: req}, nil
	})
	wrapped := rhttp.Retry(rhttp.RetryConfig{
		MaxAttempts:     3,
		RetryAllMethods: true,
		Backoff:         func(int, *http.Response) time.Duration { return 0 },
	})(rt)

	payload := []byte(`{"x":1}`)
	orig, _ := http.NewRequest(http.MethodPost, "http://example.com", bytes.NewReader(payload))
	orig.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	origBody := orig.Body

	_, _ = wrapped.RoundTrip(orig)

	if orig.Body != origBody {
		t.Fatal("RoundTrip mutated req.Body of the original request (http.RoundTripper contract)")
	}
}

func TestExponentialBackoff(t *testing.T) {
	backoff := rhttp.ExponentialBackoff(100*time.Millisecond, 1*time.Second)

	// Test exponential growth (with some tolerance for jitter)
	for attempt := 0; attempt < 5; attempt++ {
		d := backoff(attempt, nil)
		expected := 100 * time.Millisecond * (1 << attempt)
		if expected > 1*time.Second {
			expected = 1 * time.Second
		}

		// Allow 25% tolerance for jitter
		minExpected := time.Duration(float64(expected) * 0.75)
		maxExpected := time.Duration(float64(expected) * 1.25)

		if d < minExpected || d > maxExpected {
			t.Errorf("attempt %d: expected ~%v, got %v", attempt, expected, d)
		}
	}
}

func TestRetry_BackoffReceivesPreviousResponse(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body:       http.NoBody,
				Request:    req,
			}
			resp.Header.Set("Retry-After", "1")
			return resp, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     rhttp.WithRetryAfter(rhttp.ConstantBackoff(10 * time.Millisecond)),
		})),
	)

	start := time.Now()
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected the retry to honor Retry-After (~1s), waited only %v", elapsed)
	}
}

func TestRetry_BackoffCanceledClosesRequestBody(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     rhttp.ConstantBackoff(200 * time.Millisecond),
		})),
	)

	rec := &closeRecorder{Reader: strings.NewReader("payload")}
	req, _ := http.NewRequest(http.MethodPut, "http://example.com", http.NoBody)
	req.Body = rec
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("payload")), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Do(ctx, req)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded during backoff, got %v", err)
	}
	if !rec.closed {
		t.Error("request body was not closed when backoff was canceled")
	}
}
