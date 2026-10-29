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

	// OnStateChange is called after every transition. Use it to publish circuit
	// state to logs or metrics: polling State() cannot observe a transition that
	// completes within a single request, which is what the Half-Open phase does
	// at the default SuccessThreshold of 1.
	//
	// It runs on the goroutine of the request that caused the transition, with
	// the breaker's mutex released, so reading State() from inside is safe. It
	// must not block: the request cannot proceed until it returns.
	//
	// Transitions are published in order for a single request stream. Under
	// concurrency two transitions may be published in an order different from
	// the one in which they occurred, because the mutex is released first.
	OnStateChange func(from, to CircuitState)
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

// CircuitBreaker returns a middleware backed by its own breaker, created on each
// application of the middleware. The breaker itself is not returned: set
// cfg.OnStateChange to observe its transitions, or use CircuitBreakerWithState
// to get a handle to it.
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

// stateTransition is a state change captured under the breaker's mutex, to be
// published once it is released.
type stateTransition struct {
	from, to CircuitState
}

// setState moves the breaker to a new state and returns the transition for
// publication, or nil when nobody is listening. Every transition bumps the
// generation, so recordResult can discard results from requests that outlived
// the state in which they were admitted. The caller holds cb.mu.
func (cb *circuitBreaker) setState(to CircuitState) *stateTransition {
	from := cb.state
	cb.state = to
	cb.generation++

	if cb.cfg.OnStateChange == nil {
		return nil
	}
	return &stateTransition{from: from, to: to}
}

// publish invokes OnStateChange. It must run with cb.mu released: a callback
// that logs, records a metric or reads State() would otherwise deadlock.
func (cb *circuitBreaker) publish(t *stateTransition) {
	if t == nil {
		return
	}
	cb.cfg.OnStateChange(t.from, t.to)
}

// tryAdmit is allowRequest's locked section: it reports whether the request is
// admitted, the generation under which it was admitted, and the transition it
// caused, if any.
func (cb *circuitBreaker) tryAdmit() (admitted bool, gen uint64, t *stateTransition) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true, cb.generation, nil

	case CircuitOpen:
		if time.Since(cb.lastFailureTime) >= cb.cfg.ResetTimeout {
			t = cb.setState(CircuitHalfOpen)
			cb.halfOpenSuccess = 0
			cb.halfOpenInFlight = 1
			return true, cb.generation, t
		}
		return false, cb.generation, nil

	case CircuitHalfOpen:
		if cb.halfOpenInFlight < cb.cfg.MaxHalfOpenRequests {
			cb.halfOpenInFlight++
			return true, cb.generation, nil
		}
		return false, cb.generation, nil

	default:
		return true, cb.generation, nil
	}
}

// allowRequest reports whether the request is admitted and returns the
// generation under which it was admitted.
func (cb *circuitBreaker) allowRequest() (admitted bool, gen uint64) {
	admitted, gen, transition := cb.tryAdmit()
	cb.publish(transition)
	return admitted, gen
}

func (cb *circuitBreaker) recordClosedResult(isFailure bool) *stateTransition {
	if !isFailure {
		cb.failures = 0
		return nil
	}

	cb.failures++
	cb.lastFailureTime = time.Now()
	if cb.failures >= cb.cfg.FailureThreshold {
		return cb.setState(CircuitOpen)
	}
	return nil
}

func (cb *circuitBreaker) recordHalfOpenResult(isFailure bool) *stateTransition {
	if cb.halfOpenInFlight > 0 {
		cb.halfOpenInFlight--
	}

	if isFailure {
		transition := cb.setState(CircuitOpen)
		cb.lastFailureTime = time.Now()
		cb.failures = cb.cfg.FailureThreshold
		cb.halfOpenSuccess = 0
		cb.halfOpenInFlight = 0
		return transition
	}

	cb.halfOpenSuccess++
	if cb.halfOpenSuccess < cb.cfg.SuccessThreshold {
		return nil
	}

	transition := cb.setState(CircuitClosed)
	cb.failures = 0
	cb.halfOpenSuccess = 0
	cb.halfOpenInFlight = 0
	return transition
}

// applyResult is recordResult's locked section, returning the transition the
// result caused, if any.
func (cb *circuitBreaker) applyResult(resp *http.Response, err error, gen uint64) *stateTransition {
	isFailure := cb.cfg.IsFailure(resp, err)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Discard results from a bygone episode: the state under which the request
	// was admitted no longer exists, so counting it would corrupt the current
	// one (e.g. a slow Closed request closing a Half-Open circuit).
	if gen != cb.generation {
		return nil
	}

	switch cb.state {
	case CircuitClosed:
		return cb.recordClosedResult(isFailure)

	case CircuitHalfOpen:
		return cb.recordHalfOpenResult(isFailure)

	case CircuitOpen:
		// Unreachable: a request is only admitted while Closed or on the
		// transition into Half-Open, and every transition bumps the generation.
		// A result observed while the breaker sits in Open therefore carries a
		// stale generation and was already discarded above. Kept for switch
		// exhaustiveness.
		return nil

	default:
		return nil
	}
}

func (cb *circuitBreaker) recordResult(resp *http.Response, err error, gen uint64) {
	cb.publish(cb.applyResult(resp, err, gen))
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

// CircuitBreakerWithState behaves like CircuitBreaker and additionally returns
// the breaker it built, for callers that need to reach the state machine they
// just configured.
//
// To publish transitions, prefer CircuitBreakerConfig.OnStateChange: polling the
// returned State() cannot observe a transition that completes within a single
// request, which is what the Half-Open phase does at the default
// SuccessThreshold of 1.
func CircuitBreakerWithState(cfg CircuitBreakerConfig) (Middleware, *SharedCircuitBreaker) {
	shared := NewCircuitBreaker(cfg)
	mw := shared.Middleware()
	return mw, shared
}
