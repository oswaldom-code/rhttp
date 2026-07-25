package rhttp

import "net/http"

// Middleware wraps an http.RoundTripper to add behavior.
type Middleware func(http.RoundTripper) http.RoundTripper

// RoundTripperFunc adapts an ordinary function to an [http.RoundTripper],
// which makes writing a custom [Middleware] a one-liner:
//
//	func withRequestID(next http.RoundTripper) http.RoundTripper {
//		return rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
//			req.Header.Set("X-Request-ID", newID())
//			return next.RoundTrip(req)
//		})
//	}
type RoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements the [http.RoundTripper] interface.
func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// closeRequestBody releases the request body when a middleware short-circuits
// and the request will never reach the transport, honoring the RoundTripper
// contract that the body is always closed.
func closeRequestBody(req *http.Request) {
	if req.Body != nil && req.Body != http.NoBody {
		_ = req.Body.Close()
	}
}

// chain applies middleware in reverse order so the first middleware
// in the slice is the outermost wrapper (executes first).
func chain(base http.RoundTripper, mws ...Middleware) http.RoundTripper {
	rt := base
	for i := len(mws) - 1; i >= 0; i-- {
		rt = mws[i](rt)
	}
	return rt
}
