package rhttp_test

import (
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestConstantBackoff(t *testing.T) {
	backoff := rhttp.ConstantBackoff(100 * time.Millisecond)

	for attempt := 0; attempt < 10; attempt++ {
		d := backoff(attempt)
		if d != 100*time.Millisecond {
			t.Errorf("attempt %d: expected 100ms, got %v", attempt, d)
		}
	}
}

func TestLinearBackoff(t *testing.T) {
	backoff := rhttp.LinearBackoff(100*time.Millisecond, 500*time.Millisecond)

	expected := []time.Duration{
		100 * time.Millisecond, // attempt 0: 100 * 1
		200 * time.Millisecond, // attempt 1: 100 * 2
		300 * time.Millisecond, // attempt 2: 100 * 3
		400 * time.Millisecond, // attempt 3: 100 * 4
		500 * time.Millisecond, // attempt 4: 100 * 5 = max
		500 * time.Millisecond, // attempt 5: capped at max
	}

	for attempt, exp := range expected {
		d := backoff(attempt)
		if d != exp {
			t.Errorf("attempt %d: expected %v, got %v", attempt, exp, d)
		}
	}
}

func TestExponentialBackoff_Growth(t *testing.T) {
	backoff := rhttp.ExponentialBackoff(100*time.Millisecond, 10*time.Second)

	// Test exponential growth (with tolerance for jitter)
	expectedBase := []time.Duration{
		100 * time.Millisecond, // attempt 0: 100 * 2^0
		200 * time.Millisecond, // attempt 1: 100 * 2^1
		400 * time.Millisecond, // attempt 2: 100 * 2^2
		800 * time.Millisecond, // attempt 3: 100 * 2^3
	}

	for attempt, exp := range expectedBase {
		d := backoff(attempt)
		// Allow 25% tolerance for jitter
		minExpected := time.Duration(float64(exp) * 0.75)
		maxExpected := time.Duration(float64(exp) * 1.25)
		if d < minExpected || d > maxExpected {
			t.Errorf("attempt %d: expected ~%v, got %v", attempt, exp, d)
		}
	}
}

func TestExponentialBackoff_Max(t *testing.T) {
	backoff := rhttp.ExponentialBackoff(100*time.Millisecond, 500*time.Millisecond)

	// After a few attempts, should be capped at max
	d := backoff(10)
	// With jitter, should be within ±25% of 500ms
	if d > 625*time.Millisecond {
		t.Errorf("expected capped at ~500ms, got %v", d)
	}
}

func TestFibonacciBackoff(t *testing.T) {
	backoff := rhttp.FibonacciBackoff(100*time.Millisecond, 10*time.Second)

	// Fibonacci: 1, 1, 2, 3, 5, 8, 13...
	expected := []time.Duration{
		100 * time.Millisecond, // attempt 0: fib(1) = 1
		100 * time.Millisecond, // attempt 1: fib(2) = 1
		200 * time.Millisecond, // attempt 2: fib(3) = 2
		300 * time.Millisecond, // attempt 3: fib(4) = 3
		500 * time.Millisecond, // attempt 4: fib(5) = 5
		800 * time.Millisecond, // attempt 5: fib(6) = 8
	}

	for attempt, exp := range expected {
		d := backoff(attempt)
		if d != exp {
			t.Errorf("attempt %d: expected %v, got %v", attempt, exp, d)
		}
	}
}

func TestFibonacciBackoff_Max(t *testing.T) {
	backoff := rhttp.FibonacciBackoff(100*time.Millisecond, 500*time.Millisecond)

	// Should cap at 500ms
	d := backoff(10)
	if d != 500*time.Millisecond {
		t.Errorf("expected capped at 500ms, got %v", d)
	}
}

func TestDecorrelatedJitterBackoff(t *testing.T) {
	backoff := rhttp.DecorrelatedJitterBackoff(100*time.Millisecond, 10*time.Second)

	// First attempt should be base
	d0 := backoff(0)
	if d0 != 100*time.Millisecond {
		t.Errorf("attempt 0: expected 100ms, got %v", d0)
	}

	// Subsequent attempts should vary and be within bounds
	for i := 1; i < 5; i++ {
		d := backoff(i)
		// Should be positive and not exceed max
		if d <= 0 || d > 10*time.Second {
			t.Errorf("attempt %d: unexpected duration %v", i, d)
		}
	}
}

func TestExponentialBackoffFullJitter(t *testing.T) {
	backoff := rhttp.ExponentialBackoffFullJitter(100*time.Millisecond, 10*time.Second)

	for attempt := 0; attempt < 5; attempt++ {
		d := backoff(attempt)
		ceiling := 100 * time.Millisecond * (1 << attempt)
		if ceiling > 10*time.Second {
			ceiling = 10 * time.Second
		}

		// Full jitter means 0 <= d <= ceiling
		if d < 0 || d > ceiling {
			t.Errorf("attempt %d: expected 0 <= d <= %v, got %v", attempt, ceiling, d)
		}
	}
}

func TestExponentialBackoffEqualJitter(t *testing.T) {
	backoff := rhttp.ExponentialBackoffEqualJitter(100*time.Millisecond, 10*time.Second)

	for attempt := 0; attempt < 5; attempt++ {
		d := backoff(attempt)
		ceiling := 100 * time.Millisecond * (1 << attempt)
		if ceiling > 10*time.Second {
			ceiling = 10 * time.Second
		}

		// Equal jitter means ceiling/2 <= d <= ceiling
		half := ceiling / 2
		if d < half || d > ceiling {
			t.Errorf("attempt %d: expected %v <= d <= %v, got %v", attempt, half, ceiling, d)
		}
	}
}

func TestWithJitter(t *testing.T) {
	constant := rhttp.ConstantBackoff(100 * time.Millisecond)
	withJitter := rhttp.WithJitter(constant, 0.5) // 50% jitter

	// Run multiple times and check variance
	var minD, maxD time.Duration = time.Hour, 0
	for i := 0; i < 100; i++ {
		d := withJitter(0)
		if d < minD {
			minD = d
		}
		if d > maxD {
			maxD = d
		}
	}

	// With 50% jitter on 100ms, range should be 50ms - 150ms
	if minD >= 90*time.Millisecond {
		t.Errorf("min %v suggests jitter is not working", minD)
	}
	if maxD <= 110*time.Millisecond {
		t.Errorf("max %v suggests jitter is not working", maxD)
	}
}

func TestWithMax(t *testing.T) {
	linear := rhttp.LinearBackoff(100*time.Millisecond, 10*time.Second)
	capped := rhttp.WithMax(linear, 300*time.Millisecond)

	// attempt 5 would be 600ms without cap
	d := capped(5)
	if d != 300*time.Millisecond {
		t.Errorf("expected capped at 300ms, got %v", d)
	}
}

func TestWithMin(t *testing.T) {
	constant := rhttp.ConstantBackoff(10 * time.Millisecond)
	withMin := rhttp.WithMin(constant, 100*time.Millisecond)

	d := withMin(0)
	if d != 100*time.Millisecond {
		t.Errorf("expected min 100ms, got %v", d)
	}
}

func BenchmarkBackoffStrategies(b *testing.B) {
	strategies := map[string]rhttp.BackoffFunc{
		"Constant":    rhttp.ConstantBackoff(100 * time.Millisecond),
		"Linear":      rhttp.LinearBackoff(100*time.Millisecond, 10*time.Second),
		"Exponential": rhttp.ExponentialBackoff(100*time.Millisecond, 10*time.Second),
		"Fibonacci":   rhttp.FibonacciBackoff(100*time.Millisecond, 10*time.Second),
		"FullJitter":  rhttp.ExponentialBackoffFullJitter(100*time.Millisecond, 10*time.Second),
	}

	for name, backoff := range strategies {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = backoff(i % 10)
			}
		})
	}
}
