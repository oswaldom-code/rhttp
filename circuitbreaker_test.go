package rhttp_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
	"github.com/oswaldom-code/rhttp/internal"
)

func TestCircuitBreaker_ClosedState_AllowsRequests(t *testing.T) {
	var calls int32
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	time.Sleep(60 * time.Millisecond)

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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	time.Sleep(15 * time.Millisecond)

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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	time.Sleep(15 * time.Millisecond)

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
	callCount := 0
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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

func (b *blockingProbe) rt() internal.RoundTripperFunc {
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

func openCircuit(t *testing.T, c rhttp.Client, times int) {
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
	time.Sleep(15 * time.Millisecond)
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
	time.Sleep(15 * time.Millisecond)
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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	time.Sleep(15 * time.Millisecond)

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
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
	time.Sleep(15 * time.Millisecond)
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

func TestCircuitBreaker_CustomIsFailure(t *testing.T) {
	var calls int32
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
