package rhttp

import (
	"net/http"
	"time"
)

// Logger is the interface for logging HTTP requests.
// Implement this interface to integrate with your logging library.
type Logger interface {
	Log(entry LogEntry)
}

// LogEntry contains information about an HTTP request/response.
type LogEntry struct {
	// Request info
	Method string
	URL    string

	// Response info (nil values if request failed)
	StatusCode int
	Duration   time.Duration

	// Error if request failed
	Error error
}

// LoggerFunc is an adapter to allow ordinary functions to be used as Logger.
type LoggerFunc func(LogEntry)

func (f LoggerFunc) Log(entry LogEntry) {
	f(entry)
}

// LoggingConfig configures the logging middleware.
type LoggingConfig struct {
	// Logger is the logger to use. Required.
	Logger Logger

	// ShouldLog determines if a request/response should be logged.
	// If nil, all requests are logged.
	ShouldLog func(req *http.Request, resp *http.Response, err error) bool
}

// Logging returns a middleware that logs HTTP requests and responses.
func Logging(cfg LoggingConfig) Middleware {
	if cfg.Logger == nil {
		// No-op if no logger provided
		return func(next http.RoundTripper) http.RoundTripper {
			return next
		}
	}

	if cfg.ShouldLog == nil {
		cfg.ShouldLog = func(*http.Request, *http.Response, error) bool { return true }
	}

	return func(next http.RoundTripper) http.RoundTripper {
		return loggingRoundTripper{
			next: next,
			cfg:  cfg,
		}
	}
}

type loggingRoundTripper struct {
	next http.RoundTripper
	cfg  LoggingConfig
}

func (l loggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := l.next.RoundTrip(req)

	duration := time.Since(start)

	if !l.cfg.ShouldLog(req, resp, err) {
		return resp, err
	}

	entry := LogEntry{
		Method:   req.Method,
		URL:      req.URL.String(),
		Duration: duration,
		Error:    err,
	}

	if resp != nil {
		entry.StatusCode = resp.StatusCode
	}

	l.cfg.Logger.Log(entry)

	return resp, err
}
