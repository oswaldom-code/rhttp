package rhttp

import "errors"

var (
	// ErrInvalidRequest is returned when a nil request is passed to Do.
	ErrInvalidRequest = errors.New("rhttp: invalid request")

	// ErrCircuitOpen is returned when the circuit breaker is open.
	ErrCircuitOpen = errors.New("rhttp: circuit breaker is open")

	// ErrRateLimited is returned when the rate limit is exceeded and WaitOnLimit is false.
	ErrRateLimited = errors.New("rhttp: rate limit exceeded")
)
