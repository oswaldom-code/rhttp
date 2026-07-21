package rhttp

import "net/http"

// Middleware wraps an http.RoundTripper to add behavior.
type Middleware func(http.RoundTripper) http.RoundTripper

// chain applies middleware in reverse order so the first middleware
// in the slice is the outermost wrapper (executes first).
func chain(base http.RoundTripper, mws ...Middleware) http.RoundTripper {
	rt := base
	for i := len(mws) - 1; i >= 0; i-- {
		rt = mws[i](rt)
	}
	return rt
}
