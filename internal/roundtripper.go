// Package internal provides internal utilities for the rhttp package.
package internal

import "net/http"

// RoundTripperFunc is an adapter that allows ordinary functions to be used
// as http.RoundTripper. This is useful for creating mock transports in tests.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements the http.RoundTripper interface.
func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
