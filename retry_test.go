package rhttp_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
	"github.com/oswaldom-code/rhttp/internal"
)

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	var attempts int32
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
			Backoff:     func(int) time.Duration { return time.Millisecond },
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
	expectedErr := errors.New("connection refused")
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, expectedErr
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     func(int) time.Duration { return time.Millisecond },
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("connection refused")
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			return nil, errors.New("connection refused")
		}
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts:     3,
			RetryAllMethods: true,
			Backoff:         func(int) time.Duration { return time.Millisecond },
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("connection refused")
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     func(int) time.Duration { return 10 * time.Second },
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
			rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
					Backoff:     func(int) time.Duration { return time.Millisecond },
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
			Backoff:     func(int) time.Duration { return time.Millisecond },
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("connection refused")
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     func(int) time.Duration { return time.Millisecond },
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

func TestExponentialBackoff(t *testing.T) {
	backoff := rhttp.ExponentialBackoff(100*time.Millisecond, 1*time.Second)

	// Test exponential growth (with some tolerance for jitter)
	for attempt := 0; attempt < 5; attempt++ {
		d := backoff(attempt)
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
