package rhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Timeout returns a middleware that applies a timeout to requests.
// If the request's context already has a shorter deadline, it is respected.
//
// A non-positive duration is not a timeout, so the middleware falls back to a
// pass-through. That fallback is reported through OnInvalidConfig: it leaves the
// request bounded only by the caller's context, and DefaultTransport bounds
// neither dialing nor the wait for response headers.
func Timeout(d time.Duration) Middleware {
	if d <= 0 {
		reportInvalidConfig("Timeout", fmt.Sprintf(
			"duration is %v: requests are not bounded by this middleware", d))
		return func(next http.RoundTripper) http.RoundTripper {
			return next
		}
	}
	return func(next http.RoundTripper) http.RoundTripper {
		return timeoutRoundTripper{next: next, timeout: d}
	}
}

type timeoutRoundTripper struct {
	next    http.RoundTripper
	timeout time.Duration
}

func (t timeoutRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// Only apply timeout if ctx doesn't have a shorter deadline
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) <= t.timeout {
			return t.next.RoundTrip(req)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)

	req = req.WithContext(ctx)
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		cancel()
		return resp, err
	}

	if resp.Body == nil {
		cancel()
		return resp, nil
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}
