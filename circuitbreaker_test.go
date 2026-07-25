package rhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestCircuitBreaker_ClosedState_AllowsRequests(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 3,
		})),
	)

	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		resp, err := c.Do(context.Background(), req)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status: %d", resp.StatusCode)
		}
	}

	if calls != 5 {
		t.Fatalf("expected 5 calls, got %d", calls)
	}
}

func TestCircuitBreaker_OpensAfterFailureThreshold(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New("connection refused")
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 3,
			ResetTimeout:     1 * time.Hour, // Long timeout so it stays open
		})),
	)

	// First 3 calls should go through and fail
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	if calls != 3 {
		t.Fatalf("expected 3 calls before circuit opens, got %d", calls)
	}

	// 4th call should be rejected by circuit breaker
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// Transport should not have been called
	if calls != 3 {
		t.Fatalf("expected 3 calls (circuit should block), got %d", calls)
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	var calls int32
	shouldSucceed := false
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		if shouldSucceed {
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
		return nil, errors.New("connection refused")
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     50 * time.Millisecond,
		})),
	)

	// Trigger circuit open
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	// Verify circuit is open
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)
	if !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("expected circuit to be open, got %v", err)
	}

	// Wait for reset timeout
	time.Sleep(110 * time.Millisecond)

	// Now circuit should be half-open, next request goes through
	shouldSucceed = true
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("expected request to succeed in half-open state, got %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	callCount := 0
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		if callCount <= 2 {
			return nil, errors.New("connection refused")
		}
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     10 * time.Millisecond,
		})),
	)

	// Open circuit with failures
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Success in half-open should close circuit
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	// Circuit should be closed, multiple requests should work
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		resp, err := c.Do(context.Background(), req)
		if err != nil {
			t.Fatalf("expected success after circuit closed, got %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     10 * time.Millisecond,
		})),
	)

	// Open circuit
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Failure in half-open should reopen circuit
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	// Next request should be rejected (circuit reopened)
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("expected circuit to reopen after half-open failure, got %v", err)
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	// Control case: with threshold 3 and no intermediate success, the third
	// consecutive failure must open the circuit. Without this, the assertion
	// below would also pass if failures were never counted at all.
	var failCalls int32
	failRT := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&failCalls, 1)
		return nil, errors.New("connection refused")
	})
	cFail := rhttp.New(
		rhttp.WithTransport(failRT),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 3,
			ResetTimeout:     1 * time.Hour,
		})),
	)
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = cFail.Do(context.Background(), req)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	if _, err := cFail.Do(context.Background(), req); !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("control case: expected open circuit after 3 straight failures, got %v", err)
	}

	callCount := 0
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		// Fail on calls 1, 2, then succeed, then fail on 4, 5
		if callCount <= 2 || callCount >= 4 && callCount <= 5 {
			return nil, errors.New("connection refused")
		}
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 3,
			ResetTimeout:     1 * time.Hour,
		})),
	)

	// 2 failures
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	// 1 success - should reset counter
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	// 2 more failures - should not open circuit (counter was reset)
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	// Circuit should still be closed
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	// Should not be ErrCircuitOpen (might be connection refused or success)
	if errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatal("circuit should not be open - success should have reset failure count")
	}
}

func TestCircuitBreaker_5xxStatusCountsAsFailure(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: http.StatusInternalServerError, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     1 * time.Hour,
		})),
	)

	// 2 calls with 500 should open circuit
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	// 3rd call should be blocked
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("expected circuit to open after 5xx responses, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestCircuitBreaker_ThreadSafety(t *testing.T) {
	var calls int64
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt64(&calls, 1)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 100,
		})),
	)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
			_, _ = c.Do(context.Background(), req)
		}()
	}

	wg.Wait()

	if calls != 100 {
		t.Fatalf("expected 100 concurrent calls to succeed, got %d", calls)
	}
}

// blockingProbe is a transport that fails while half-open is off (to open the
// circuit), then blocks each admitted request inside the transport until
// release is closed, signaling entry on entered. It lets a test hold half-open
// probes in flight to observe concurrent gating.
type blockingProbe struct {
	halfOpen atomic.Bool
	entered  chan struct{}
	release  chan struct{}
	probes   int32
}

func newBlockingProbe() *blockingProbe {
	return &blockingProbe{
		entered: make(chan struct{}, 16),
		release: make(chan struct{}),
	}
}

func (b *blockingProbe) rt() rhttp.RoundTripperFunc {
	return func(req *http.Request) (*http.Response, error) {
		if !b.halfOpen.Load() {
			return nil, errors.New("connection refused")
		}
		atomic.AddInt32(&b.probes, 1)
		b.entered <- struct{}{}
		<-b.release
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	}
}

func openCircuit(t *testing.T, c *rhttp.Client, times int) {
	t.Helper()
	for i := 0; i < times; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}
}

// Regression: half-open must admit only MaxHalfOpenRequests probes (default 1),
// not every concurrent request. The mutex is released between allowRequest and
// recordResult, so a naive implementation lets all concurrent requests through.
func TestCircuitBreaker_HalfOpenAdmitsSingleProbeByDefault(t *testing.T) {
	bp := newBlockingProbe()
	c := rhttp.New(
		rhttp.WithTransport(bp.rt()),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     10 * time.Millisecond,
		})),
	)

	openCircuit(t, c, 2)
	time.Sleep(60 * time.Millisecond)
	bp.halfOpen.Store(true)

	// One probe transitions to half-open and blocks inside the transport.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}()
	<-bp.entered // probe is now in flight; state is Half-Open with one probe

	// While the probe is in flight, further requests must be rejected.
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, err := c.Do(context.Background(), req)
		if !errors.Is(err, rhttp.ErrCircuitOpen) {
			t.Fatalf("expected ErrCircuitOpen for concurrent probe, got %v", err)
		}
	}

	close(bp.release)
	wg.Wait()

	if got := atomic.LoadInt32(&bp.probes); got != 1 {
		t.Fatalf("expected exactly 1 probe to reach the transport, got %d", got)
	}
}

func TestCircuitBreaker_HalfOpenRespectsMaxHalfOpenRequests(t *testing.T) {
	const maxProbes = 3
	bp := newBlockingProbe()
	c := rhttp.New(
		rhttp.WithTransport(bp.rt()),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold:    2,
			ResetTimeout:        10 * time.Millisecond,
			MaxHalfOpenRequests: maxProbes,
		})),
	)

	openCircuit(t, c, 2)
	time.Sleep(60 * time.Millisecond)
	bp.halfOpen.Store(true)

	// Admit maxProbes concurrent probes; hold them all in flight.
	var wg sync.WaitGroup
	for i := 0; i < maxProbes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
			_, _ = c.Do(context.Background(), req)
		}()
	}
	for i := 0; i < maxProbes; i++ {
		<-bp.entered
	}

	// One more must be rejected: the half-open budget is exhausted.
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	if _, err := c.Do(context.Background(), req); !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen once %d probes are in flight, got %v", maxProbes, err)
	}

	close(bp.release)
	wg.Wait()

	if got := atomic.LoadInt32(&bp.probes); got != maxProbes {
		t.Fatalf("expected %d probes to reach the transport, got %d", maxProbes, got)
	}
}

func TestCircuitBreaker_OneSuccessDoesNotCloseWithThreshold(t *testing.T) {
	var succeed atomic.Bool
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if succeed.Load() {
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
		return nil, errors.New("connection refused")
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     10 * time.Millisecond,
			SuccessThreshold: 2,
		})),
	)

	openCircuit(t, c, 2)
	time.Sleep(60 * time.Millisecond)

	// First half-open probe succeeds (1 of 2 required).
	succeed.Store(true)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	if _, err := c.Do(context.Background(), req); err != nil {
		t.Fatalf("first probe should be admitted, got %v", err)
	}

	// Still half-open: a failing probe must reopen the circuit immediately.
	succeed.Store(false)
	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	req, _ = http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)
	if !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("one success must not close the circuit when SuccessThreshold=2, got %v", err)
	}
}

func TestCircuitBreaker_ClosesAfterSuccessThreshold(t *testing.T) {
	var succeed atomic.Bool
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		if succeed.Load() {
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
		return nil, errors.New("connection refused")
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     10 * time.Millisecond,
			SuccessThreshold: 2,
		})),
	)

	openCircuit(t, c, 2)
	time.Sleep(60 * time.Millisecond)
	succeed.Store(true)

	// Two sequential half-open successes close the circuit.
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		if _, err := c.Do(context.Background(), req); err != nil {
			t.Fatalf("half-open probe %d should be admitted, got %v", i+1, err)
		}
	}

	// Closed: a concurrent burst is no longer gated to a single probe.
	before := atomic.LoadInt32(&calls)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
			if _, err := c.Do(context.Background(), req); err != nil {
				t.Errorf("expected success after circuit closed, got %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls) - before; got != 10 {
		t.Fatalf("expected 10 calls to reach the transport once closed, got %d", got)
	}
}

func TestCircuitBreaker_MiddlewareApplicationsAreIndependent(t *testing.T) {
	mw := rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     time.Hour,
	})

	backendErr := errors.New("backend down")
	failing := rhttp.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, backendErr
	})
	healthy := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})

	chainA := mw(failing)
	chainB := mw(healthy)

	reqA, _ := http.NewRequest(http.MethodGet, "http://a.example", http.NoBody)
	if _, err := chainA.RoundTrip(reqA); !errors.Is(err, backendErr) {
		t.Fatalf("chain A reached the wrong transport (next overwritten by chain B): err=%v", err)
	}

	reqB, _ := http.NewRequest(http.MethodGet, "http://b.example", http.NoBody)
	resp, err := chainB.RoundTrip(reqB)
	if errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatal("chain B's circuit opened due to chain A's failures: shared state")
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("chain B did not reach its own transport: resp=%v err=%v", resp, err)
	}
}

func TestCircuitBreaker_SharedInstanceSharesState(t *testing.T) {
	shared := rhttp.NewCircuitBreaker(rhttp.CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     time.Hour,
	})

	backendErr := errors.New("backend down")
	failing := rhttp.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, backendErr
	})
	healthy := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})

	chainA := shared.Middleware()(failing)
	chainB := shared.Middleware()(healthy)

	reqA, _ := http.NewRequest(http.MethodGet, "http://a.example", http.NoBody)
	if _, err := chainA.RoundTrip(reqA); !errors.Is(err, backendErr) {
		t.Fatalf("chain A should reach its failing transport, got %v", err)
	}

	reqB, _ := http.NewRequest(http.MethodGet, "http://b.example", http.NoBody)
	if _, err := chainB.RoundTrip(reqB); !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("shared breaker: chain B should see the circuit opened by chain A, got %v", err)
	}
}

func TestCircuitBreaker_ClientCancellationsDoNotOpenCircuit(t *testing.T) {
	rt := rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     time.Hour,
	})(rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: context.Canceled}
	}))

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	for i := 0; i < 5; i++ {
		_, _ = rt.RoundTrip(req)
	}

	if _, err := rt.RoundTrip(req); errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatal("client cancellations opened the circuit against a healthy upstream")
	}
}

func TestCircuitBreaker_TimeoutsOpenCircuit(t *testing.T) {
	rt := rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     time.Hour,
	})(rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: context.DeadlineExceeded}
	}))

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	for i := 0; i < 3; i++ {
		_, _ = rt.RoundTrip(req)
	}

	if _, err := rt.RoundTrip(req); !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatal("timeouts should count as failures and open the circuit")
	}
}

func TestCircuitBreaker_CustomIsFailure(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		// Return 429 which is not a 5xx
		return &http.Response{StatusCode: http.StatusTooManyRequests, Request: req}, nil
	})

	// Custom IsFailure that treats 429 as failure
	customIsFailure := func(resp *http.Response, err error) bool {
		if err != nil {
			return true
		}
		if resp != nil && (resp.StatusCode >= 500 || resp.StatusCode == 429) {
			return true
		}
		return false
	}

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     1 * time.Hour,
			IsFailure:        customIsFailure,
		})),
	)

	// 2 calls with 429 should open circuit
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	// 3rd call should be blocked
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("expected circuit to open with custom IsFailure, got %v", err)
	}
}

func TestCircuitBreaker_IsFailureMayCallState(t *testing.T) {
	var scb *rhttp.SharedCircuitBreaker
	scb = rhttp.NewCircuitBreaker(rhttp.CircuitBreakerConfig{
		FailureThreshold: 2,
		IsFailure: func(_ *http.Response, err error) bool {
			// A user callback that inspects the breaker must not deadlock.
			_ = scb.State()
			return err != nil
		},
	})

	rt := rhttp.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(scb.Middleware()),
	)

	done := make(chan struct{})
	go func() {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("IsFailure calling State() deadlocked recordResult")
	}
}

// Regression (C2): a slow request admitted while Closed must not have its late
// result counted against a later Half-Open episode. Without generation gating,
// its success runs recordHalfOpenResult, closing the circuit and freeing the
// real probe's budget.
func TestCircuitBreaker_StaleResultDoesNotCloseHalfOpen(t *testing.T) {
	enteredA := make(chan struct{})
	relA := make(chan struct{})
	enteredC := make(chan struct{})
	relC := make(chan struct{})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Header.Get("X-Role") {
		case "fail":
			return nil, errors.New("connection refused")
		case "slow-closed":
			close(enteredA)
			<-relA
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		case "probe":
			close(enteredC)
			<-relC
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		default:
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
	})

	scb := rhttp.NewCircuitBreaker(rhttp.CircuitBreakerConfig{
		FailureThreshold: 1,
		ResetTimeout:     10 * time.Millisecond,
	})
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(scb.Middleware()),
	)

	do := func(role string) {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		req.Header.Set("X-Role", role)
		_, _ = c.Do(context.Background(), req)
	}

	var wg sync.WaitGroup

	// 1. Admit a slow request while Closed; hold it in flight.
	doneA := make(chan struct{})
	wg.Add(1)
	go func() { defer wg.Done(); do("slow-closed"); close(doneA) }()
	<-enteredA

	// 2. A failure opens the circuit (threshold 1).
	do("fail")
	if got := scb.State(); got != rhttp.CircuitOpen {
		t.Fatalf("expected Open after failure, got %v", got)
	}

	// 3. After the reset timeout, admit a probe; hold it in flight (Half-Open).
	time.Sleep(60 * time.Millisecond)
	wg.Add(1)
	go func() { defer wg.Done(); do("probe") }()
	<-enteredC
	if got := scb.State(); got != rhttp.CircuitHalfOpen {
		t.Fatalf("expected Half-Open with a probe in flight, got %v", got)
	}

	// 4. The stale Closed-era request completes successfully and is recorded.
	close(relA)
	<-doneA

	// 5. The circuit must stay Half-Open and the probe budget must remain taken.
	if got := scb.State(); got != rhttp.CircuitHalfOpen {
		t.Fatalf("stale Closed result closed the circuit: state=%v", got)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	req.Header.Set("X-Role", "check")
	if _, err := c.Do(context.Background(), req); !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("stale result freed the half-open budget: got %v", err)
	}

	close(relC)
	wg.Wait()
}

func TestCircuitState_String(t *testing.T) {
	cases := map[rhttp.CircuitState]string{
		rhttp.CircuitClosed:    "closed",
		rhttp.CircuitOpen:      "open",
		rhttp.CircuitHalfOpen:  "half-open",
		rhttp.CircuitState(99): "unknown",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("state %d: expected %q, got %q", int(state), want, got)
		}
	}
}

func TestCircuitBreaker_OpenClosesRequestBody(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.CircuitBreaker(rhttp.CircuitBreakerConfig{
			FailureThreshold: 1,
			ResetTimeout:     time.Hour,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	rec := &closeRecorder{Reader: strings.NewReader("payload")}
	req, _ = http.NewRequest(http.MethodPut, "http://example.com", http.NoBody)
	req.Body = rec
	_, err := c.Do(context.Background(), req)

	if !errors.Is(err, rhttp.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if !rec.closed {
		t.Error("request body was not closed on circuit-open short-circuit")
	}
}

func TestSharedCircuitBreaker_StateObservesTransitions(t *testing.T) {
	bp := newBlockingProbe()
	shared := rhttp.NewCircuitBreaker(rhttp.CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     10 * time.Millisecond,
	})
	c := rhttp.New(
		rhttp.WithTransport(bp.rt()),
		rhttp.WithMiddleware(shared.Middleware()),
	)

	if got := shared.State(); got != rhttp.CircuitClosed {
		t.Fatalf("expected initial state closed, got %v", got)
	}

	openCircuit(t, c, 2)
	if got := shared.State(); got != rhttp.CircuitOpen {
		t.Fatalf("expected open after %d failures, got %v", 2, got)
	}

	time.Sleep(60 * time.Millisecond)
	bp.halfOpen.Store(true)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}()

	<-bp.entered
	if got := shared.State(); got != rhttp.CircuitHalfOpen {
		t.Fatalf("expected half-open while the probe is in flight, got %v", got)
	}

	close(bp.release)
	<-done
	if got := shared.State(); got != rhttp.CircuitClosed {
		t.Fatalf("expected closed after a successful probe, got %v", got)
	}
}
