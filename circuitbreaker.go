package rhttp

import (
	"net/http"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreakerConfig configures the circuit breaker middleware.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit.
	FailureThreshold int

	// ResetTimeout is how long to wait in Open state before transitioning to Half-Open.
	ResetTimeout time.Duration

	// IsFailure determines if a response/error should count as a failure.
	// If nil, any error or 5xx status code is considered a failure.
	IsFailure func(resp *http.Response, err error) bool

	// MaxHalfOpenRequests is the number of probe requests allowed concurrently
	// while in Half-Open state. If <= 0, defaults to 1 (single-probe).
	MaxHalfOpenRequests int

	// SuccessThreshold is the number of consecutive successful probes required
	// in Half-Open state to close the circuit. If <= 0, defaults to 1.
	SuccessThreshold int
}

func DefaultIsFailure(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp != nil && resp.StatusCode >= 500 {
		return true
	}
	return false
}

// CircuitBreaker returns a middleware that implements the circuit breaker pattern.
func CircuitBreaker(cfg CircuitBreakerConfig) Middleware {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.IsFailure == nil {
		cfg.IsFailure = DefaultIsFailure
	}
	if cfg.MaxHalfOpenRequests <= 0 {
		cfg.MaxHalfOpenRequests = 1
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 1
	}

	cb := &circuitBreaker{
		cfg:   cfg,
		state: CircuitClosed,
	}

	return func(next http.RoundTripper) http.RoundTripper {
		cb.next = next
		return cb
	}
}

type circuitBreaker struct {
	next http.RoundTripper
	cfg  CircuitBreakerConfig

	mu               sync.Mutex
	state            CircuitState
	failures         int
	lastFailureTime  time.Time
	halfOpenInFlight int
	halfOpenSuccess  int
}

func (cb *circuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		if time.Since(cb.lastFailureTime) >= cb.cfg.ResetTimeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenSuccess = 0
			cb.halfOpenInFlight = 1
			return true
		}
		return false

	case CircuitHalfOpen:
		if cb.halfOpenInFlight < cb.cfg.MaxHalfOpenRequests {
			cb.halfOpenInFlight++
			return true
		}
		return false

	default:
		return true
	}
}

func (cb *circuitBreaker) recordClosedResult(isFailure bool) {
	if !isFailure {
		cb.failures = 0
		return
	}

	cb.failures++
	cb.lastFailureTime = time.Now()
	if cb.failures >= cb.cfg.FailureThreshold {
		cb.state = CircuitOpen
	}
}

func (cb *circuitBreaker) recordHalfOpenResult(isFailure bool) {
	if cb.halfOpenInFlight > 0 {
		cb.halfOpenInFlight--
	}

	if isFailure {
		cb.state = CircuitOpen
		cb.lastFailureTime = time.Now()
		cb.failures = cb.cfg.FailureThreshold
		cb.halfOpenSuccess = 0
		return
	}

	cb.halfOpenSuccess++
	if cb.halfOpenSuccess < cb.cfg.SuccessThreshold {
		return
	}

	cb.state = CircuitClosed
	cb.failures = 0
	cb.halfOpenSuccess = 0
	cb.halfOpenInFlight = 0
}

func (cb *circuitBreaker) recordResult(resp *http.Response, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	isFailure := cb.cfg.IsFailure(resp, err)

	switch cb.state {
	case CircuitClosed:
		cb.recordClosedResult(isFailure)

	case CircuitHalfOpen:
		cb.recordHalfOpenResult(isFailure)

	case CircuitOpen:
		// Unreachable: allowRequest rejects requests while Open, so a result
		// is never recorded in this state. Handled to keep the switch exhaustive.
	}
}

func (cb *circuitBreaker) RoundTrip(req *http.Request) (*http.Response, error) {
	if !cb.allowRequest() {
		return nil, ErrCircuitOpen
	}

	resp, err := cb.next.RoundTrip(req)

	cb.recordResult(resp, err)

	return resp, err
}

// State returns the current state of the circuit breaker.
// Useful for monitoring and testing.
func (cb *circuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
