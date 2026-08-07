package rhttp_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestOnInvalidConfig_ReportsLostProtections(t *testing.T) {
	var reported []string

	rhttp.OnInvalidConfig = func(component, reason string) {
		if reason == "" {
			t.Errorf("%s was reported without a reason", component)
		}
		reported = append(reported, component)
	}
	t.Cleanup(func() { rhttp.OnInvalidConfig = nil })

	rhttp.Timeout(0)
	rhttp.Timeout(-time.Second)
	rhttp.RateLimit(rhttp.RateLimitConfig{})
	rhttp.NewTokenBucket(0, 10)
	rhttp.NewTokenBucket(10, 0)

	// A nil Recorder or Logger means "observability not configured", which is a
	// legitimate default: nothing is lost and nothing must be reported.
	rhttp.Metrics(rhttp.MetricsConfig{})
	rhttp.Logging(rhttp.LoggingConfig{})

	// A valid configuration is silent too.
	rhttp.Timeout(time.Second)
	rhttp.NewTokenBucket(10, 1)

	want := []string{"Timeout", "Timeout", "RateLimit", "TokenBucket", "TokenBucket"}

	if len(reported) != len(want) {
		t.Fatalf("reported = %v, want %v", reported, want)
	}
	for i := range want {
		if reported[i] != want[i] {
			t.Errorf("report %d = %q, want %q (full: %v)", i, reported[i], want[i], reported)
		}
	}
}

func TestOnInvalidConfig_NilKeepsTheFallbackSilent(t *testing.T) {
	if rhttp.OnInvalidConfig != nil {
		t.Fatal("OnInvalidConfig must default to nil")
	}

	probe := &probeRoundTripper{}

	if rhttp.Timeout(0)(probe) != http.RoundTripper(probe) {
		t.Error("Timeout(0) must still fall back to a pass-through")
	}
	if !rhttp.NewTokenBucket(0, 10).TryAcquire() {
		t.Error("an invalid bucket must still fall back to not limiting")
	}
}

// probeRoundTripper is a comparable innermost transport, so a middleware can be
// checked for being a pass-through: Go func values are not comparable.
type probeRoundTripper struct{}

func (*probeRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}
