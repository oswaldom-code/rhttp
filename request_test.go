package rhttp_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oswaldom-code/rhttp"
)

func TestRequestBuilder_Get(t *testing.T) {
	var capturedReq *http.Request
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	resp, err := c.R().Get("http://example.com/api")

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
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusCreated, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	resp, err := c.R().
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
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = c.R().
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
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = c.R().
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
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = c.R().
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
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		capturedBody, _ = io.ReadAll(req.Body)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	payload := map[string]string{"name": "test", "value": "123"}
	_, _ = c.R().
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
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		body, _ := io.ReadAll(req.Body)
		capturedBody = string(body)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = c.R().
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
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = c.R().
		SetAuthToken("my-token-123").
		Get("http://example.com/api")

	expected := "Bearer my-token-123"
	if capturedReq.Header.Get("Authorization") != expected {
		t.Errorf("expected Authorization=%s, got %s", expected, capturedReq.Header.Get("Authorization"))
	}
}

func TestRequestBuilder_SetBasicAuth(t *testing.T) {
	var capturedReq *http.Request
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, _ = c.R().
		SetBasicAuth("user", "pass").
		Get("http://example.com/api")

	auth := capturedReq.Header.Get("Authorization")
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if auth != want {
		t.Errorf("expected Authorization %q, got %q", want, auth)
	}
}

func TestRequestBuilder_Timeout(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		// Respect context cancellation
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(200 * time.Millisecond):
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		}
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	_, err := c.R().
		SetTimeout(50 * time.Millisecond).
		Get("http://example.com/api")

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestRequestBuilder_SetTimeoutBodyReadableAfterReturn(t *testing.T) {
	const head, tail = "first-chunk-", "second-chunk"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter is not a Flusher")
			return
		}
		_, _ = io.WriteString(w, head)
		fl.Flush()
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(w, tail)
	}))
	defer srv.Close()

	c := rhttp.New()

	resp, err := c.R().
		SetTimeout(5 * time.Second).
		Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body after builder execute returned: %v", err)
	}
	if string(body) != head+tail {
		t.Fatalf("expected body %q, got %q", head+tail, body)
	}
}

func TestRequestBuilder_Context(t *testing.T) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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

	_, err := c.R().
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
			rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				capturedMethod = req.Method
				return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
			})

			c := rhttp.New(rhttp.WithTransport(rt))
			_, _ = m.fn(c.R(), "http://example.com")

			if capturedMethod != m.expect {
				t.Errorf("expected %s, got %s", m.expect, capturedMethod)
			}
		})
	}
}

type opaqueReader struct{ r io.Reader }

func (o *opaqueReader) Read(p []byte) (int, error) { return o.r.Read(p) }

func TestRequestBuilder_ReaderBodyIsRetryable(t *testing.T) {
	attempts := 0
	var bodies []string
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		b, _ := io.ReadAll(req.Body)
		bodies = append(bodies, string(b))
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts:     3,
			RetryAllMethods: true,
			Backoff:         rhttp.ConstantBackoff(0),
		})),
	)

	_, _ = c.R().
		SetBody(&opaqueReader{r: strings.NewReader("payload")}).
		Post("http://example.com")

	if attempts != 3 {
		t.Fatalf("opaque reader body disabled retries: got %d attempts, want 3", attempts)
	}
	for i, b := range bodies {
		if b != "payload" {
			t.Errorf("attempt %d body = %q, want %q", i+1, b, "payload")
		}
	}
}

func BenchmarkRequestBuilder_Simple(b *testing.B) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.R().Get("http://example.com")
	}
}

func BenchmarkRequestBuilder_WithOptions(b *testing.B) {
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = c.R().
			SetHeader("Authorization", "Bearer token").
			SetQueryParam("page", "1").
			SetPathParam("id", "123").
			Get("http://example.com/users/{id}")
	}
}

func TestRequestBuilder_SecondExecuteResendsFullBody(t *testing.T) {
	var bodies []string
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, string(data))
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})

	c := rhttp.New(rhttp.WithTransport(rt))
	rb := c.R().SetBodyBytes([]byte("payload"))

	for i := 0; i < 2; i++ {
		resp, err := rb.Post("http://example.com")
		if err != nil {
			t.Fatalf("execute %d: unexpected error: %v", i, err)
		}
		resp.Body.Close()
	}

	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Fatalf("expected both executions to send the full body, got %q", bodies)
	}
}

func TestRequestBuilder_LargeBodyStreamsWithoutRetry(t *testing.T) {
	// One byte over the 10 MB buffering limit forces the streaming path.
	const size = 10<<20 + 1

	var attempts int32
	var received int64
	var sawGetBody bool
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		sawGetBody = req.GetBody != nil
		n, err := io.Copy(io.Discard, req.Body)
		if err != nil {
			return nil, err
		}
		atomic.StoreInt64(&received, n)
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody, Request: req}, nil
	})

	c := rhttp.New(
		rhttp.WithTransport(rt),
		rhttp.WithMiddleware(rhttp.Retry(rhttp.RetryConfig{
			MaxAttempts: 3,
			Backoff:     rhttp.ConstantBackoff(time.Millisecond),
		})),
	)

	opaque := &nonReplayableReader{r: bytes.NewReader(make([]byte, size))}
	resp, err := c.R().
		SetBody(opaque).
		Put("http://example.com/upload")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if sawGetBody {
		t.Error("expected GetBody to be nil on the streaming path")
	}
	if received != size {
		t.Errorf("expected the transport to receive %d bytes, got %d", size, received)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("expected a single attempt for a non-replayable streamed body, got %d", got)
	}
}

func TestRequestBuilder_MalformedURL(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})
	c := rhttp.New(rhttp.WithTransport(rt))

	_, err := c.R().Get("http://exa mple.com/api")
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if calls != 0 {
		t.Errorf("expected the transport to never run, got %d calls", calls)
	}
}

func TestRequestBuilder_SetBodyJSONMarshalError(t *testing.T) {
	var calls int32
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})
	c := rhttp.New(rhttp.WithTransport(rt))

	_, err := c.R().
		SetBodyJSON(make(chan int)).
		Post("http://example.com")
	if err == nil {
		t.Fatal("expected marshal error for unsupported JSON type")
	}
	if calls != 0 {
		t.Errorf("expected the transport to never run, got %d calls", calls)
	}
}

func TestRequestBuilder_SetBodyXML(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody string
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		body, _ := io.ReadAll(req.Body)
		capturedBody = string(body)
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})
	c := rhttp.New(rhttp.WithTransport(rt))

	type User struct {
		Name string `xml:"name"`
	}
	_, err := c.R().
		SetBodyXML(User{Name: "John"}).
		Post("http://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := capturedReq.Header.Get("Content-Type"); got != "application/xml" {
		t.Errorf("expected Content-Type=application/xml, got %s", got)
	}
	if !strings.Contains(capturedBody, "<name>John</name>") {
		t.Errorf("unexpected XML body: %s", capturedBody)
	}
}

func TestRequestBuilder_SetBodyXMLMarshalError(t *testing.T) {
	c := rhttp.New(rhttp.WithTransport(rhttp.RoundTripperFunc(
		func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
		})))

	_, err := c.R().
		SetBodyXML(map[string]string{"k": "v"}).
		Post("http://example.com")
	if err == nil {
		t.Fatal("expected marshal error: xml does not support maps")
	}
}

func TestRequestBuilder_ExecuteCustomMethod(t *testing.T) {
	var capturedReq *http.Request
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})
	c := rhttp.New(rhttp.WithTransport(rt))

	_, err := c.R().Execute("TRACE", "http://example.com/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReq.Method != "TRACE" {
		t.Errorf("expected method TRACE, got %s", capturedReq.Method)
	}
}

func TestRequestBuilder_PathParamIsEscaped(t *testing.T) {
	var capturedReq *http.Request
	rt := rhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: http.StatusOK, Request: req}, nil
	})
	c := rhttp.New(rhttp.WithTransport(rt))

	_, err := c.R().
		SetPathParam("id", "a/b c").
		Get("http://example.com/items/{id}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "http://example.com/items/a%2Fb%20c"
	if got := capturedReq.URL.String(); got != want {
		t.Errorf("expected escaped path param URL %s, got %s", want, got)
	}
}
