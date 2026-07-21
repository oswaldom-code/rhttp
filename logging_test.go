package rhttp_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
	"github.com/oswaldom-code/rhttp/internal"
)

func TestLogging_LogsSuccessfulRequest(t *testing.T) {
	var captured rhttp.LogEntry
	logger := rhttp.LoggerFunc(func(entry rhttp.LogEntry) {
		captured = entry
	})

	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Logging(rhttp.LoggingConfig{
			Logger: logger,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/path", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Method != http.MethodGet {
		t.Errorf("expected method GET, got %s", captured.Method)
	}
	if captured.URL != "http://example.com/path" {
		t.Errorf("expected URL http://example.com/path, got %s", captured.URL)
	}
	if captured.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", captured.StatusCode)
	}
	if captured.Error != nil {
		t.Errorf("expected no error, got %v", captured.Error)
	}
	if captured.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestLogging_LogsFailedRequest(t *testing.T) {
	var captured rhttp.LogEntry
	logger := rhttp.LoggerFunc(func(entry rhttp.LogEntry) {
		captured = entry
	})

	expectedErr := errors.New("connection refused")
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, expectedErr
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Logging(rhttp.LoggingConfig{
			Logger: logger,
		})),
	)

	req, _ := http.NewRequest(http.MethodPost, "http://example.com/api", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Method != http.MethodPost {
		t.Errorf("expected method POST, got %s", captured.Method)
	}
	if captured.StatusCode != 0 {
		t.Errorf("expected status 0 on error, got %d", captured.StatusCode)
	}
	if captured.Error != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, captured.Error)
	}
}

func TestLogging_MeasuresDuration(t *testing.T) {
	var captured rhttp.LogEntry
	logger := rhttp.LoggerFunc(func(entry rhttp.LogEntry) {
		captured = entry
	})

	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		time.Sleep(50 * time.Millisecond)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Logging(rhttp.LoggingConfig{
			Logger: logger,
		})),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	if captured.Duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", captured.Duration)
	}
}

func TestLogging_ShouldLogFilters(t *testing.T) {
	var logCount int
	logger := rhttp.LoggerFunc(func(entry rhttp.LogEntry) {
		logCount++
	})

	callCount := 0
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		if callCount%2 == 0 {
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
		return &http.Response{StatusCode: http.StatusInternalServerError, Request: req}, nil
	})

	// Only log errors (5xx)
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Logging(rhttp.LoggingConfig{
			Logger: logger,
			ShouldLog: func(req *http.Request, resp *http.Response, err error) bool {
				return err != nil || (resp != nil && resp.StatusCode >= 500)
			},
		})),
	)

	// Make 4 requests: 500, 200, 500, 200
	for i := 0; i < 4; i++ {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
		_, _ = c.Do(context.Background(), req)
	}

	// Only 500s should be logged
	if logCount != 2 {
		t.Errorf("expected 2 logged requests (only errors), got %d", logCount)
	}
}

func TestLogging_NilLoggerIsNoOp(t *testing.T) {
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	// Should not panic with nil logger
	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Logging(rhttp.LoggingConfig{
			Logger: nil,
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

func TestLogging_ThreadSafety(t *testing.T) {
	var mu sync.Mutex
	var entries []rhttp.LogEntry
	logger := rhttp.LoggerFunc(func(entry rhttp.LogEntry) {
		mu.Lock()
		entries = append(entries, entry)
		mu.Unlock()
	})

	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Logging(rhttp.LoggingConfig{
			Logger: logger,
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
	count := len(entries)
	mu.Unlock()

	if count != 100 {
		t.Errorf("expected 100 log entries, got %d", count)
	}
}
