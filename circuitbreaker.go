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

// String returns the lowercase name of the state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

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
		return Classify(err).Kind != ErrKindCanceled
	}
	return resp != nil && resp.StatusCode >= 500
}

// newCircuitBreaker applies defaults and returns a circuit-breaker state machine.
func newCircuitBreaker(cfg CircuitBreakerConfig) *circuitBreaker {
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

	return &circuitBreaker{cfg: cfg, state: CircuitClosed}
}

func CircuitBreaker(cfg CircuitBreakerConfig) Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return circuitBreakerRoundTripper{next: next, cb: newCircuitBreaker(cfg)}
	}
}

type circuitBreaker struct {
	cfg CircuitBreakerConfig

	mu               sync.Mutex
	state            CircuitState
	generation       uint64
	failures         int
	lastFailureTime  time.Time
	halfOpenInFlight int
	halfOpenSuccess  int
}

// allowRequest reports whether the request is admitted and returns the
// generation under which it was admitted. Every state transition bumps the
// generation, so recordResult can discard results from requests that outlived
// the state in which they were admitted.
func (cb *circuitBreaker) allowRequest() (admitted bool, gen uint64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true, cb.generation

	case CircuitOpen:
		if time.Since(cb.lastFailureTime) >= cb.cfg.ResetTimeout {
			cb.state = CircuitHalfOpen
			cb.generation++
			cb.halfOpenSuccess = 0
			cb.halfOpenInFlight = 1
			return true, cb.generation
		}
		return false, cb.generation

	case CircuitHalfOpen:
		if cb.halfOpenInFlight < cb.cfg.MaxHalfOpenRequests {
			cb.halfOpenInFlight++
			return true, cb.generation
		}
		return false, cb.generation

	default:
		return true, cb.generation
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
		cb.generation++
	}
}

func (cb *circuitBreaker) recordHalfOpenResult(isFailure bool) {
	if cb.halfOpenInFlight > 0 {
		cb.halfOpenInFlight--
	}

	if isFailure {
		cb.state = CircuitOpen
		cb.generation++
		cb.lastFailureTime = time.Now()
		cb.failures = cb.cfg.FailureThreshold
		cb.halfOpenSuccess = 0
		cb.halfOpenInFlight = 0
		return
	}

	cb.halfOpenSuccess++
	if cb.halfOpenSuccess < cb.cfg.SuccessThreshold {
		return
	}

	cb.state = CircuitClosed
	cb.generation++
	cb.failures = 0
	cb.halfOpenSuccess = 0
	cb.halfOpenInFlight = 0
}

func (cb *circuitBreaker) recordResult(resp *http.Response, err error, gen uint64) {
	isFailure := cb.cfg.IsFailure(resp, err)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Discard results from a bygone episode: the state under which the request
	// was admitted no longer exists, so counting it would corrupt the current
	// one (e.g. a slow Closed request closing a Half-Open circuit).
	if gen != cb.generation {
		return
	}

	switch cb.state {
	case CircuitClosed:
		cb.recordClosedResult(isFailure)

	case CircuitHalfOpen:
		cb.recordHalfOpenResult(isFailure)

	case CircuitOpen:
		// Unreachable: a request is only admitted while Closed or on the
		// transition into Half-Open, and every transition bumps the generation.
		// A result observed while the breaker sits in Open therefore carries a
		// stale generation and was already discarded above. Kept for switch
		// exhaustiveness.
	}
}

// State returns the current state of the circuit breaker.
// Useful for monitoring and testing.
func (cb *circuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

type circuitBreakerRoundTripper struct {
	next http.RoundTripper
	cb   *circuitBreaker
}

func (rt circuitBreakerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	allowed, gen := rt.cb.allowRequest()
	if !allowed {
		closeRequestBody(req)
		return nil, ErrCircuitOpen
	}

	resp, err := rt.next.RoundTrip(req)

	rt.cb.recordResult(resp, err, gen)

	return resp, err
}

// SharedCircuitBreaker is a circuit breaker whose state can be shared across
// multiple middleware applications or clients. Unlike the CircuitBreaker
// middleware, which creates an independent breaker per application, all
// middleware derived from the same SharedCircuitBreaker observe the same state.
type SharedCircuitBreaker struct {
	cb *circuitBreaker
}

// NewCircuitBreaker creates a SharedCircuitBreaker with the given configuration,
// applying defaults for any zero-valued fields. Use it when several clients must
// trip together against the same dependency.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *SharedCircuitBreaker {
	return &SharedCircuitBreaker{cb: newCircuitBreaker(cfg)}
}

// Middleware returns a Middleware backed by this shared breaker. Applying it to
// multiple clients makes them share a single circuit state.
func (s *SharedCircuitBreaker) Middleware() Middleware {
	return func(next http.RoundTripper) http.RoundTripper {
		return circuitBreakerRoundTripper{next: next, cb: s.cb}
	}
}

// State returns the current state of the shared circuit breaker.
func (s *SharedCircuitBreaker) State() CircuitState {
	return s.cb.State()
}
