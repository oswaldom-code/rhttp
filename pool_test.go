package rhttp_test

import (
	"sync"
	"testing"

	"github.com/oswaldom-code/rhttp"
)

func TestBufferPool_GetAndPut(t *testing.T) {
	buf := rhttp.GetBuffer()
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}

	buf.WriteString("test data")
	if buf.Len() != 9 {
		t.Errorf("expected length 9, got %d", buf.Len())
	}

	rhttp.PutBuffer(buf)

	// Get another buffer - should be reset
	buf2 := rhttp.GetBuffer()
	if buf2.Len() != 0 {
		t.Errorf("expected reset buffer with length 0, got %d", buf2.Len())
	}
	rhttp.PutBuffer(buf2)
}

func TestBufferPool_NilSafe(_ *testing.T) {
	// Should not panic
	rhttp.PutBuffer(nil)
}

func TestBufferPool_Concurrent(_ *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := rhttp.GetBuffer()
			buf.WriteString("concurrent test")
			rhttp.PutBuffer(buf)
		}()
	}
	wg.Wait()
}

func TestResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		status   int
		expected bool
	}{
		{200, true},
		{201, true},
		{204, true},
		{299, true},
		{300, false},
		{400, false},
		{500, false},
	}

	for _, tt := range tests {
		r := &rhttp.Response{StatusCode: tt.status}
		if r.IsSuccess() != tt.expected {
			t.Errorf("IsSuccess(%d) = %v, want %v", tt.status, r.IsSuccess(), tt.expected)
		}
	}
}

func TestResponse_IsError(t *testing.T) {
	tests := []struct {
		status   int
		expected bool
	}{
		{200, false},
		{399, false},
		{400, true},
		{404, true},
		{500, true},
		{503, true},
	}

	for _, tt := range tests {
		r := &rhttp.Response{StatusCode: tt.status}
		if r.IsError() != tt.expected {
			t.Errorf("IsError(%d) = %v, want %v", tt.status, r.IsError(), tt.expected)
		}
	}
}

func TestResponse_IsServerError(t *testing.T) {
	tests := []struct {
		status   int
		expected bool
	}{
		{499, false},
		{500, true},
		{502, true},
		{503, true},
	}

	for _, tt := range tests {
		r := &rhttp.Response{StatusCode: tt.status}
		if r.IsServerError() != tt.expected {
			t.Errorf("IsServerError(%d) = %v, want %v", tt.status, r.IsServerError(), tt.expected)
		}
	}
}

func TestResponse_IsClientError(t *testing.T) {
	tests := []struct {
		status   int
		expected bool
	}{
		{399, false},
		{400, true},
		{404, true},
		{499, true},
		{500, false},
	}

	for _, tt := range tests {
		r := &rhttp.Response{StatusCode: tt.status}
		if r.IsClientError() != tt.expected {
			t.Errorf("IsClientError(%d) = %v, want %v", tt.status, r.IsClientError(), tt.expected)
		}
	}
}

func BenchmarkBufferPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := rhttp.GetBuffer()
		buf.WriteString("benchmark test data")
		rhttp.PutBuffer(buf)
	}
}

func BenchmarkBufferPool_NoPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 0, 4096)
		buf = append(buf, "benchmark test data"...)
		_ = buf
	}
}
