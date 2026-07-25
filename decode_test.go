package rhttp_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/oswaldom-code/rhttp"
)

type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}

func jsonResponse(status int, body string) (*http.Response, *closeRecorder) {
	rec := &closeRecorder{Reader: strings.NewReader(body)}
	return &http.Response{StatusCode: status, Body: rec}, rec
}

func TestDecodeJSON_Success(t *testing.T) {
	resp, rec := jsonResponse(http.StatusOK, `{"name":"John","age":30}`)

	var out struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	if err := rhttp.DecodeJSON(resp, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "John" || out.Age != 30 {
		t.Errorf("unexpected decode result: %+v", out)
	}
	if !rec.closed {
		t.Error("body was not closed")
	}
}

func TestDecodeJSON_ErrorStatusDoesNotDecode(t *testing.T) {
	resp, rec := jsonResponse(http.StatusInternalServerError, `{"name":"John"}`)

	var out struct {
		Name string `json:"name"`
	}
	err := rhttp.DecodeJSON(resp, &out)
	if err == nil {
		t.Fatal("expected error for status 500")
	}
	if out.Name != "" {
		t.Errorf("expected no decode on error status, got %+v", out)
	}
	if !rec.closed {
		t.Error("body was not closed")
	}
}

func TestDecodeJSON_RedirectStatusIsError(t *testing.T) {
	resp, rec := jsonResponse(http.StatusMovedPermanently, "")

	var out any
	if err := rhttp.DecodeJSON(resp, &out); err == nil {
		t.Fatal("expected error for status 301")
	}
	if !rec.closed {
		t.Error("body was not closed")
	}
}

func TestDecodeJSON_MalformedBodyIsError(t *testing.T) {
	resp, rec := jsonResponse(http.StatusOK, `{"name":`)

	var out struct {
		Name string `json:"name"`
	}
	if err := rhttp.DecodeJSON(resp, &out); err == nil {
		t.Fatal("expected decode error for malformed JSON")
	}
	if !rec.closed {
		t.Error("body was not closed")
	}
}
