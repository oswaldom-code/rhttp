package rhttp_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/oswaldom-code/rhttp"
)

func TestClient_Do(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Request:    req,
		}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestClient_Do_NilRequest(t *testing.T) {
	c := rhttp.New()

	_, err := c.Do(context.Background(), nil)
	if err != rhttp.ErrInvalidRequest {
		t.Fatalf("expected ErrInvalidRequest, got: %v", err)
	}
}

func TestClient_MiddlewareChain(t *testing.T) {
	var order []int

	mw1 := func(next http.RoundTripper) http.RoundTripper {
		return rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			order = append(order, 1)
			return next.RoundTrip(req)
		})
	}

	mw2 := func(next http.RoundTripper) http.RoundTripper {
		return rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			order = append(order, 2)
			return next.RoundTrip(req)
		})
	}

	base := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		order = append(order, 0)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(base),
		rhttp.WithMiddleware(mw1, mw2),
	)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	_, _ = c.Do(context.Background(), req)

	// mw1 should execute first, then mw2, then base
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 0 {
		t.Fatalf("unexpected middleware order: %v", order)
	}
}

func TestDo_NilContextDoesNotPanic(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	c := rhttp.New(rhttp.WithTransport(rt))

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", http.NoBody)
	resp, err := c.Do(nil, req) //nolint:staticcheck // nil ctx is the case under test
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
