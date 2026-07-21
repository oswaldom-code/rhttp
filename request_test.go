package rhttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
	"github.com/oswaldom-code/rhttp/internal"
)

func TestRequestBuilder_Get(t *testing.T) {
	var capturedReq *http.Request
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	resp, err := rhttp.R(c).Get("http://example.com/api")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedReq.Method != http.MethodGet {
		t.Errorf("expected GET, got %s", capturedReq.Method)
	}
	if capturedReq.URL.String() != "http://example.com/api" {
		t.Errorf("expected http://example.com/api, got %s", capturedReq.URL.String())
	}
}

func TestRequestBuilder_Post(t *testing.T) {
	var capturedReq *http.Request
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusCreated, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	resp, err := rhttp.R(c).
		SetBodyString("test body").
		Post("http://example.com/api")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	if capturedReq.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedReq.Method)
	}
}

func TestRequestBuilder_Headers(t *testing.T) {
	var capturedReq *http.Request
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = rhttp.R(c).
		SetHeader("X-Custom", "value1").
		SetHeaders(map[string]string{
			"X-Another": "value2",
			"X-Third":   "value3",
		}).
		AddHeader("X-Multi", "a").
		AddHeader("X-Multi", "b").
		SetContentType("application/json").
		SetAccept("application/json").
		SetUserAgent("test-agent").
		Get("http://example.com")

	if capturedReq.Header.Get("X-Custom") != "value1" {
		t.Errorf("expected X-Custom=value1, got %s", capturedReq.Header.Get("X-Custom"))
	}
	if capturedReq.Header.Get("X-Another") != "value2" {
		t.Errorf("expected X-Another=value2, got %s", capturedReq.Header.Get("X-Another"))
	}
	if capturedReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %s", capturedReq.Header.Get("Content-Type"))
	}
	if capturedReq.Header.Get("Accept") != "application/json" {
		t.Errorf("expected Accept=application/json, got %s", capturedReq.Header.Get("Accept"))
	}
	if capturedReq.Header.Get("User-Agent") != "test-agent" {
		t.Errorf("expected User-Agent=test-agent, got %s", capturedReq.Header.Get("User-Agent"))
	}

	multiVals := capturedReq.Header.Values("X-Multi")
	if len(multiVals) != 2 || multiVals[0] != "a" || multiVals[1] != "b" {
		t.Errorf("expected X-Multi=[a,b], got %v", multiVals)
	}
}

func TestRequestBuilder_QueryParams(t *testing.T) {
	var capturedReq *http.Request
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = rhttp.R(c).
		SetQueryParam("page", "1").
		SetQueryParams(map[string]string{
			"limit": "10",
			"sort":  "desc",
		}).
		AddQueryParam("filter", "active").
		AddQueryParam("filter", "verified").
		Get("http://example.com/users")

	query := capturedReq.URL.Query()
	if query.Get("page") != "1" {
		t.Errorf("expected page=1, got %s", query.Get("page"))
	}
	if query.Get("limit") != "10" {
		t.Errorf("expected limit=10, got %s", query.Get("limit"))
	}
	if query.Get("sort") != "desc" {
		t.Errorf("expected sort=desc, got %s", query.Get("sort"))
	}

	filters := query["filter"]
	if len(filters) != 2 {
		t.Errorf("expected 2 filter values, got %d", len(filters))
	}
}

func TestRequestBuilder_PathParams(t *testing.T) {
	var capturedReq *http.Request
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = rhttp.R(c).
		SetPathParam("org", "acme").
		SetPathParams(map[string]string{
			"repo": "api",
			"id":   "123",
		}).
		Get("http://example.com/{org}/{repo}/issues/{id}")

	expected := "http://example.com/acme/api/issues/123"
	if capturedReq.URL.String() != expected {
		t.Errorf("expected %s, got %s", expected, capturedReq.URL.String())
	}
}

func TestRequestBuilder_SetBodyJSON(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		capturedBody, _ = io.ReadAll(req.Body)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	payload := map[string]string{"name": "test", "value": "123"}
	_, _ = rhttp.R(c).
		SetBodyJSON(payload).
		Post("http://example.com/api")

	if capturedReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %s", capturedReq.Header.Get("Content-Type"))
	}

	var decoded map[string]string
	if err := json.Unmarshal(capturedBody, &decoded); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if decoded["name"] != "test" || decoded["value"] != "123" {
		t.Errorf("unexpected body: %v", decoded)
	}
}

func TestRequestBuilder_SetBodyForm(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody string
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		body, _ := io.ReadAll(req.Body)
		capturedBody = string(body)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = rhttp.R(c).
		SetBodyForm(map[string]string{
			"username": "test",
			"password": "secret",
		}).
		Post("http://example.com/login")

	if capturedReq.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("expected Content-Type=application/x-www-form-urlencoded, got %s", capturedReq.Header.Get("Content-Type"))
	}

	if !strings.Contains(capturedBody, "username=test") {
		t.Errorf("expected body to contain username=test, got %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "password=secret") {
		t.Errorf("expected body to contain password=secret, got %s", capturedBody)
	}
}

func TestRequestBuilder_SetAuthToken(t *testing.T) {
	var capturedReq *http.Request
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = rhttp.R(c).
		SetAuthToken("my-token-123").
		Get("http://example.com/api")

	expected := "Bearer my-token-123"
	if capturedReq.Header.Get("Authorization") != expected {
		t.Errorf("expected Authorization=%s, got %s", expected, capturedReq.Header.Get("Authorization"))
	}
}

func TestRequestBuilder_SetBasicAuth(t *testing.T) {
	var capturedReq *http.Request
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = rhttp.R(c).
		SetBasicAuth("user", "pass").
		Get("http://example.com/api")

	auth := capturedReq.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("expected Authorization to start with 'Basic ', got %s", auth)
	}
}

func TestRequestBuilder_Timeout(t *testing.T) {
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		// Respect context cancellation
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(200 * time.Millisecond):
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, err := rhttp.R(c).
		SetTimeout(50 * time.Millisecond).
		Get("http://example.com/api")

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected deadline exceeded error, got %v", err)
	}
}

func TestRequestBuilder_Context(t *testing.T) {
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		// Respect context cancellation
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(200 * time.Millisecond):
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := rhttp.R(c).
		Context(ctx).
		Get("http://example.com/api")

	if err == nil {
		t.Fatal("expected context timeout error")
	}
}

func TestRequestBuilder_AllMethods(t *testing.T) {
	methods := []struct {
		name   string
		fn     func(*rhttp.RequestBuilder, string) (*http.Response, error)
		expect string
	}{
		{"Get", func(rb *rhttp.RequestBuilder, url string) (*http.Response, error) { return rb.Get(url) }, "GET"},
		{"Post", func(rb *rhttp.RequestBuilder, url string) (*http.Response, error) { return rb.Post(url) }, "POST"},
		{"Put", func(rb *rhttp.RequestBuilder, url string) (*http.Response, error) { return rb.Put(url) }, "PUT"},
		{"Patch", func(rb *rhttp.RequestBuilder, url string) (*http.Response, error) { return rb.Patch(url) }, "PATCH"},
		{"Delete", func(rb *rhttp.RequestBuilder, url string) (*http.Response, error) { return rb.Delete(url) }, "DELETE"},
		{"Head", func(rb *rhttp.RequestBuilder, url string) (*http.Response, error) { return rb.Head(url) }, "HEAD"},
		{"Options", func(rb *rhttp.RequestBuilder, url string) (*http.Response, error) { return rb.Options(url) }, "OPTIONS"},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			var capturedMethod string
			rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				capturedMethod = req.Method
				return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
			})

			c := rhttp.New(rhttp.WithTransport(rt))
			_, _ = m.fn(rhttp.R(c), "http://example.com")

			if capturedMethod != m.expect {
				t.Errorf("expected %s, got %s", m.expect, capturedMethod)
			}
		})
	}
}

func BenchmarkRequestBuilder_Simple(b *testing.B) {
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = rhttp.R(c).Get("http://example.com")
	}
}

func BenchmarkRequestBuilder_WithOptions(b *testing.B) {
	rt := internal.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = rhttp.R(c).
			SetHeader("Authorization", "Bearer token").
			SetQueryParam("page", "1").
			SetPathParam("id", "123").
			Get("http://example.com/users/{id}")
	}
}
