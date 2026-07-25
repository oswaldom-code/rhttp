package rhttp

import (
	"net/http"
	"time"
)

// MetricsRecorder is the interface for recording HTTP client metrics.
// Implement this interface to integrate with your metrics system (Prometheus, StatsD, etc.).
type MetricsRecorder interface {
	RecordRequest(event MetricEvent)
}

// MetricEvent contains metrics data for a single HTTP request.
type MetricEvent struct {
	// Request info
	Method string
	Host   string
	Path   string

	// Response info
	StatusCode    int
	Duration      time.Duration
	BytesSent     int64
	BytesReceived int64

	// Error info
	Error   error
	Success bool
}

// MetricsRecorderFunc is an adapter to allow ordinary functions as MetricsRecorder.
type MetricsRecorderFunc func(MetricEvent)

func (f MetricsRecorderFunc) RecordRequest(event MetricEvent) {
	f(event)
}

// MetricsConfig configures the metrics middleware.
type MetricsConfig struct {
	// Recorder is the metrics recorder. Required.
	Recorder MetricsRecorder

	// PathNormalizer maps a request path to the value emitted as MetricEvent.Path.
	// It exists to bound label cardinality: raw paths like /users/8f3a/orders/2941
	// would create one time series per ID. If nil, Path is emitted empty; to collapse
	// high-cardinality segments, provide a function that returns a template such as
	// /users/:id/orders/:id.
	PathNormalizer func(path string) string
}

// Metrics returns a middleware that records HTTP client metrics.
func Metrics(cfg MetricsConfig) Middleware {
	if cfg.Recorder == nil {
		return func(next http.RoundTripper) http.RoundTripper {
			return next
		}
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return metricsRoundTripper{
			next:           next,
			recorder:       cfg.Recorder,
			pathNormalizer: cfg.PathNormalizer,
		}
	}
}

type metricsRoundTripper struct {
	next           http.RoundTripper
	recorder       MetricsRecorder
	pathNormalizer func(path string) string
}

func (m metricsRoundTripper) normalizePath(path string) string {
	if m.pathNormalizer == nil {
		return ""
	}
	return m.pathNormalizer(path)
}

func (m metricsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := m.next.RoundTrip(req)

	duration := time.Since(start)

	event := MetricEvent{
		Method:   req.Method,
		Host:     req.URL.Host,
		Path:     m.normalizePath(req.URL.Path),
		Duration: duration,
		Error:    err,
		Success:  err == nil && resp != nil && resp.StatusCode < 500,
	}

	if req.ContentLength > 0 {
		event.BytesSent = req.ContentLength
	}

	if resp != nil {
		event.StatusCode = resp.StatusCode
		if resp.ContentLength > 0 {
			event.BytesReceived = resp.ContentLength
		}
	}

	m.recorder.RecordRequest(event)

	return resp, err
}
