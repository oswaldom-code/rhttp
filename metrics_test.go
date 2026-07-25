package rhttp_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestMetrics_RecordsSuccessfulRequest(t *testing.T) {
	var captured rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		captured = event
	})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 1024,
			Request:       req,
		}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder:       recorder,
			PathNormalizer: func(p string) string { return p },
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://api.example.com/users", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Method != http.MethodGet {
		t.Errorf("expected method GET, got %s", captured.Method)
	}
	if captured.Host != "api.example.com" {
		t.Errorf("expected host api.example.com, got %s", captured.Host)
	}
	if captured.Path != "/users" {
		t.Errorf("expected path /users, got %s", captured.Path)
	}
	if captured.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", captured.StatusCode)
	}
	if !captured.Success {
		t.Error("expected Success to be true")
	}
	if captured.Error != nil {
		t.Errorf("expected no error, got %v", captured.Error)
	}
	if captured.Duration <= 0 {
		t.Error("expected positive duration")
	}
	if captured.BytesReceived != 1024 {
		t.Errorf("expected 1024 bytes received, got %d", captured.BytesReceived)
	}
}

func TestMetrics_NilPathNormalizerEmitsEmptyPath(t *testing.T) {
	var captured rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		captured = event
	})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: recorder,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://api.example.com/users/8f3a/orders/2941", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Path != "" {
		t.Errorf("expected empty path without normalizer, got %q", captured.Path)
	}
	if captured.Host != "api.example.com" {
		t.Errorf("expected host to still be emitted, got %q", captured.Host)
	}
}

func TestMetrics_PathNormalizerTransformsPath(t *testing.T) {
	var captured rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		captured = event
	})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	normalizer := func(p string) string {
		if strings.HasPrefix(p, "/users/") {
			return "/users/:id"
		}
		return p
	}

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder:       recorder,
			PathNormalizer: normalizer,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://api.example.com/users/8f3a/profile", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Path != "/users/:id" {
		t.Errorf("expected normalized path /users/:id, got %q", captured.Path)
	}
}

func TestMetrics_RecordsFailedRequest(t *testing.T) {
	var captured rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		captured = event
	})

	expectedErr := errors.New("connection refused")
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, expectedErr
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: recorder,
		})),
	)

	req, _ := http.NewRequest(http.MethodPost, "http://api.example.com/data", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Success {
		t.Error("expected Success to be false on error")
	}
	if captured.Error != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, captured.Error)
	}
	if captured.StatusCode != 0 {
		t.Errorf("expected status 0 on error, got %d", captured.StatusCode)
	}
}

func TestMetrics_5xxIsNotSuccess(t *testing.T) {
	var captured rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		captured = event
	})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: recorder,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Success {
		t.Error("expected Success to be false for 5xx")
	}
	if captured.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", captured.StatusCode)
	}
}

func TestMetrics_4xxIsSuccess(t *testing.T) {
	var captured rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		captured = event
	})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: recorder,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	// 4xx is considered "success" from transport perspective (request completed)
	if !captured.Success {
		t.Error("expected Success to be true for 4xx (transport succeeded)")
	}
}

func TestMetrics_NilRecorderIsNoOp(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: nil,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMetrics_RecordsBytesSent(t *testing.T) {
	var captured rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		captured = event
	})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: recorder,
		})),
	)

	body := bytes.NewReader([]byte("test payload"))
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", body)
	req.ContentLength = int64(body.Len())
	_, _ = c.Do(context.Background(), req)

	if captured.BytesSent != 12 {
		t.Errorf("expected 12 bytes sent, got %d", captured.BytesSent)
	}
}

func TestMetrics_MeasuresDuration(t *testing.T) {
	var captured rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		captured = event
	})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		time.Sleep(50 * time.Millisecond)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: recorder,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", captured.Duration)
	}
}

func TestMetrics_ThreadSafety(t *testing.T) {
	var mu sync.Mutex
	var events []rhttp.MetricEvent
	recorder := rhttp.MetricsRecorderFunc(func(event rhttp.MetricEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Metrics(rhttp.MetricsConfig{
			Recorder: recorder,
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

	mu.Lock()
	count := len(events)
	mu.Unlock()

	if count != 100 {
		t.Errorf("expected 100 metric events, got %d", count)
	}
}
