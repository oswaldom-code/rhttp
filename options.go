package rhttp

import "net/http"

// Option configures a Client.
type Option func(*config)

type config struct {
	transport  http.RoundTripper
	middleware []Middleware
}

func defaultConfig() *config {
	return &config{
		transport: DefaultTransport(),
	}
}

// WithTransport sets a custom http.RoundTripper.
//
// A nil transport is discarded and DefaultTransport is kept, so no protection is
// lost. Unlike the middleware constructors, this case is not reported through
// OnInvalidConfig.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *config) {
		if rt != nil {
			c.transport = rt
		}
	}
}

// WithMiddleware appends middleware to the chain.
func WithMiddleware(mw ...Middleware) Option {
	return func(c *config) {
		c.middleware = append(c.middleware, mw...)
	}
}
