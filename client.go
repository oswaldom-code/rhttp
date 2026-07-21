package rhttp

import (
	"context"
	"net/http"
)

// Client defines the interface for executing HTTP requests.
type Client interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

type client struct {
	rt http.RoundTripper
}

// New creates a new Client with the given options.
func New(opts ...Option) Client {
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

	return &client{rt: rt}
}

// Do executes the request with the configured middleware chain.
func (c *client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, ErrInvalidRequest
	}

	req = req.Clone(ctx)
	return c.rt.RoundTrip(req)
}
