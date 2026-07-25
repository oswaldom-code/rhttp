package rhttp_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestTimeout_RequestCompletesBeforeTimeout(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Timeout(5*time.Second)),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestTimeout_RequestExceedsTimeout(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(500 * time.Millisecond):
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Timeout(50*time.Millisecond)),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestTimeout_NonPositiveDurationIsNoOp(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		t.Run(d.String(), func(t *testing.T) {
			var hadDeadline bool
			rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				_, hadDeadline = req.Context().Deadline()
				return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
			})

			c := rhttp.New(
				rhttp.WithTransport(rt),
				rhttp.WithMiddleware(rhttp.Timeout(d)),
			)

			req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
			resp, err := c.Do(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("unexpected status: %d", resp.StatusCode)
			}
			if hadDeadline {
				t.Fatal("expected no deadline for non-positive timeout")
			}
		})
	}
}

func TestTimeout_RespectsExistingShorterDeadline(t *testing.T) {
	var capturedDeadline time.Time
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedDeadline, _ = req.Context().Deadline()
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Timeout(10*time.Second)),
	)

	// Context with 100ms deadline (shorter than middleware's 10s)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", http.NoBody)
	expectedDeadline, _ := ctx.Deadline()

	_, _ = c.Do(ctx, req)

	// The captured deadline should match the original context's deadline
	if !capturedDeadline.Equal(expectedDeadline) {
		t.Fatalf("expected deadline %v, got %v", expectedDeadline, capturedDeadline)
	}
}

func TestTimeout_StreamingBodyReadableAfterReturn(t *testing.T) {
	const head, tail = "first-chunk-", "second-chunk"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter is not a Flusher")
			return
		}
		_, _ = io.WriteString(w, head)
		fl.Flush()
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(w, tail)
	}))
	defer srv.Close()

	c := rhttp.New(
		rhttp.WithMiddleware(rhttp.Timeout(5 * time.Second)),
	)

	req, _ := http.NewRequest(http.MethodGet, srv.URL, http.NoBody)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading streamed body after RoundTrip returned: %v", err)
	}
	if string(body) != head+tail {
		t.Fatalf("expected body %q, got %q", head+tail, body)
	}
}

func TestTimeout_AppliesWhenExistingDeadlineLonger(t *testing.T) {
	var capturedCtx context.Context
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedCtx = req.Context()
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Timeout(100*time.Millisecond)),
	)

	// Context with 10s deadline (longer than middleware's 100ms)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", http.NoBody)

	start := time.Now()
	_, _ = c.Do(ctx, req)

	deadline, ok := capturedCtx.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}

	// Deadline should be ~100ms from start, not 10s
	timeUntilDeadline := time.Until(deadline)
	if timeUntilDeadline > 150*time.Millisecond {
		t.Fatalf("expected deadline ~100ms from now, got %v (started at %v)", deadline, start)
	}
}
