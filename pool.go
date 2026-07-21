package rhttp

import (
	"bytes"
	"sync"
)

// BufferPool provides reusable byte buffers to reduce allocations.
var BufferPool = &sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

// GetBuffer retrieves a buffer from the pool.
func GetBuffer() *bytes.Buffer {
	buf, _ := BufferPool.Get().(*bytes.Buffer) //nolint:errcheck // type is guaranteed by pool's New func
	buf.Reset()
	return buf
}

// PutBuffer returns a buffer to the pool.
func PutBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	// Don't pool oversized buffers (>64KB) to prevent memory bloat
	if buf.Cap() > 65536 {
		return
	}
	buf.Reset()
	BufferPool.Put(buf)
}

// responsePool provides reusable response wrappers.
var responsePool = &sync.Pool{
	New: func() any {
		return &Response{}
	},
}

// Response wraps http.Response with pooling support and convenience methods.
type Response struct {
	StatusCode    int
	Headers       map[string][]string
	Body          []byte
	ContentLength int64
	pooled        bool
}

// Reset clears the response for reuse.
func (r *Response) Reset() {
	r.StatusCode = 0
	r.Headers = nil
	r.Body = nil
	r.ContentLength = 0
	r.pooled = false
}

// Release returns the response to the pool.
// After calling Release, the Response must not be used.
func (r *Response) Release() {
	if r == nil || !r.pooled {
		return
	}
	r.Reset()
	responsePool.Put(r)
}

// acquireResponse gets a response from the pool.
// Currently unused but kept for future RequestBuilder enhancements.
func acquireResponse() *Response { //nolint:unused
	r, _ := responsePool.Get().(*Response) //nolint:errcheck // type is guaranteed by pool's New func
	r.Reset()
	r.pooled = true
	return r
}

// IsSuccess returns true if status code is 2xx.
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsError returns true if status code is 4xx or 5xx.
func (r *Response) IsError() bool {
	return r.StatusCode >= 400
}

// IsServerError returns true if status code is 5xx.
func (r *Response) IsServerError() bool {
	return r.StatusCode >= 500
}

// IsClientError returns true if status code is 4xx.
func (r *Response) IsClientError() bool {
	return r.StatusCode >= 400 && r.StatusCode < 500
}
