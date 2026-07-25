package rhttp

import (
	"io"
	"net/http"
	"time"
)

// RetryConfig configures the retry middleware.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the first one).
	MaxAttempts int

	// Backoff returns the duration to wait before the nth retry (0-indexed).
	// It receives the response of the attempt that triggered the retry (nil if
	// it produced no response). If nil, exponential backoff is used.
	Backoff BackoffFunc

	// IsRetryable determines if a request should be retried based on the response and error.
	// If nil, default retry logic is used.
	IsRetryable func(resp *http.Response, err error) bool

	// RetryAllMethods if true, retries all HTTP methods including non-idempotent ones.
	// Default is false (only retry idempotent methods).
	RetryAllMethods bool
}

// Retry returns a middleware that retries failed requests.
func Retry(cfg RetryConfig) Middleware {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.Backoff == nil {
		cfg.Backoff = ExponentialBackoff(100*time.Millisecond, 10*time.Second)
	}
	if cfg.IsRetryable == nil {
		cfg.IsRetryable = DefaultIsRetryable
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return retryRoundTripper{
			next: next,
			cfg:  cfg,
		}
	}
}

type retryRoundTripper struct {
	next http.RoundTripper
	cfg  RetryConfig
}

func (r retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !r.canRetry(req) {
		return r.next.RoundTrip(req)
	}

	var resp *http.Response
	var err error

	for attempt := 0; attempt < r.cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			if err := r.waitBackoff(req, attempt, resp); err != nil {
				closeRequestBody(req)
				return nil, err
			}
		}

		attemptReq, prepErr := r.prepareRequest(req, attempt)
		if prepErr != nil {
			closeRequestBody(req)
			return nil, prepErr
		}

		resp, err = r.next.RoundTrip(attemptReq)

		if !r.cfg.IsRetryable(resp, err) {
			return resp, err
		}

		if attempt < r.cfg.MaxAttempts-1 {
			drainAndClose(resp)
		}
	}

	return resp, err
}

func (r retryRoundTripper) canRetry(req *http.Request) bool {
	if !r.cfg.RetryAllMethods && !isIdempotent(req.Method) {
		return false
	}

	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

func (r retryRoundTripper) prepareRequest(req *http.Request, attempt int) (*http.Request, error) {
	// The first attempt uses the request as-is: Do already cloned it, so the
	// caller's request is never mutated.
	if attempt == 0 {
		return req, nil
	}

	attemptReq := req.Clone(req.Context())

	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		attemptReq.Body = body
	}

	return attemptReq, nil
}

func (r retryRoundTripper) waitBackoff(req *http.Request, attempt int, prev *http.Response) error {
	timer := time.NewTimer(r.cfg.Backoff(attempt-1, prev))
	defer timer.Stop()

	select {
	case <-req.Context().Done():
		return req.Context().Err()
	case <-timer.C:
		return nil
	}
}

const maxDrainBytes = 256 << 10

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
	_ = resp.Body.Close()
}

// isIdempotent returns true for HTTP methods that are safe to retry.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// DefaultIsRetryable returns true for transient errors and retryable status codes.
func DefaultIsRetryable(resp *http.Response, err error) bool {
	if err != nil {
		return Classify(err).Kind.IsRetryable()
	}
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
