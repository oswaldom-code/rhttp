package rhttp

import (
	"context"
	"net/http"
)

// Client executes HTTP requests through the configured middleware chain.
// It is safe for concurrent use by multiple goroutines.
type Client struct {
	rt http.RoundTripper
}

// New creates a new Client with the given options.
func New(opts ...Option) *Client {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	rt := cfg.transport
	if rt == nil {
		rt = DefaultTransport()
	}

	if len(cfg.middleware) > 0 {
		rt = chain(rt, cfg.middleware...)
	}

	return &Client{rt: rt}
}

// Do executes the request with the configured middleware chain.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, ErrInvalidRequest
	}
	if ctx == nil {
		ctx = context.Background()
	}

	req = req.Clone(ctx)
	return c.rt.RoundTrip(req)
}
