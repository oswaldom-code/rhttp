package rhttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RequestBuilder provides a fluent interface for building HTTP requests.
//
// A builder is meant for a single request and is not safe for concurrent use.
// Bodies set from bytes (SetBodyBytes, SetBodyString, SetBodyJSON, SetBodyXML,
// SetBodyForm) survive re-execution; a body set from a reader via SetBody is
// consumed by the first execution.
type RequestBuilder struct {
	client      *Client
	ctx         context.Context
	method      string
	url         string
	headers     http.Header
	queryParams url.Values
	pathParams  map[string]string
	body        io.Reader
	bodyBytes   []byte
	timeout     time.Duration
	err         error
}

// R creates a new RequestBuilder bound to the client.
// Query and path parameter maps are initialized lazily on first use.
func (c *Client) R() *RequestBuilder {
	return &RequestBuilder{
		client:  c,
		ctx:     context.Background(),
		headers: make(http.Header),
	}
}

// Context sets the context for the request.
func (rb *RequestBuilder) Context(ctx context.Context) *RequestBuilder {
	rb.ctx = ctx
	return rb
}

// SetTimeout sets a timeout for this specific request.
func (rb *RequestBuilder) SetTimeout(d time.Duration) *RequestBuilder {
	rb.timeout = d
	return rb
}

// SetHeader sets a single header.
func (rb *RequestBuilder) SetHeader(key, value string) *RequestBuilder {
	rb.headers.Set(key, value)
	return rb
}

// SetHeaders sets multiple headers from a map.
func (rb *RequestBuilder) SetHeaders(headers map[string]string) *RequestBuilder {
	for k, v := range headers {
		rb.headers.Set(k, v)
	}
	return rb
}

// AddHeader adds a header value (allows multiple values for same key).
func (rb *RequestBuilder) AddHeader(key, value string) *RequestBuilder {
	rb.headers.Add(key, value)
	return rb
}

// SetContentType sets the Content-Type header.
func (rb *RequestBuilder) SetContentType(contentType string) *RequestBuilder {
	return rb.SetHeader("Content-Type", contentType)
}

// SetAccept sets the Accept header.
func (rb *RequestBuilder) SetAccept(accept string) *RequestBuilder {
	return rb.SetHeader("Accept", accept)
}

// SetUserAgent sets the User-Agent header.
func (rb *RequestBuilder) SetUserAgent(ua string) *RequestBuilder {
	return rb.SetHeader("User-Agent", ua)
}

// SetAuthToken sets a Bearer token in the Authorization header.
func (rb *RequestBuilder) SetAuthToken(token string) *RequestBuilder {
	return rb.SetHeader("Authorization", "Bearer "+token)
}

// SetBasicAuth sets Basic authentication.
func (rb *RequestBuilder) SetBasicAuth(username, password string) *RequestBuilder {
	rb.headers.Set("Authorization", "Basic "+basicAuth(username, password))
	return rb
}

func (rb *RequestBuilder) ensureQueryParams() {
	if rb.queryParams == nil {
		rb.queryParams = make(url.Values)
	}
}

// SetQueryParam sets a single query parameter.
func (rb *RequestBuilder) SetQueryParam(key, value string) *RequestBuilder {
	rb.ensureQueryParams()
	rb.queryParams.Set(key, value)
	return rb
}

// SetQueryParams sets multiple query parameters from a map.
func (rb *RequestBuilder) SetQueryParams(params map[string]string) *RequestBuilder {
	rb.ensureQueryParams()
	for k, v := range params {
		rb.queryParams.Set(k, v)
	}
	return rb
}

// AddQueryParam adds a query parameter (allows multiple values for same key).
func (rb *RequestBuilder) AddQueryParam(key, value string) *RequestBuilder {
	rb.ensureQueryParams()
	rb.queryParams.Add(key, value)
	return rb
}

func (rb *RequestBuilder) ensurePathParams() {
	if rb.pathParams == nil {
		rb.pathParams = make(map[string]string)
	}
}

// SetPathParam sets a path parameter to be replaced in the URL.
// Example: SetPathParam("id", "123") replaces {id} in "/users/{id}".
func (rb *RequestBuilder) SetPathParam(key, value string) *RequestBuilder {
	rb.ensurePathParams()
	rb.pathParams[key] = value
	return rb
}

// SetPathParams sets multiple path parameters from a map.
func (rb *RequestBuilder) SetPathParams(params map[string]string) *RequestBuilder {
	rb.ensurePathParams()
	for k, v := range params {
		rb.pathParams[k] = v
	}
	return rb
}

// SetBody sets the request body from a reader.
//
// The reader is buffered up to 10 MB so the body can be rewound and the request
// retried. If the body exceeds 10 MB it is streamed instead: the request is sent
// once and is not retried, since the reader cannot be replayed.
func (rb *RequestBuilder) SetBody(body io.Reader) *RequestBuilder {
	rb.body = body
	return rb
}

// SetBodyBytes sets the request body from bytes.
func (rb *RequestBuilder) SetBodyBytes(body []byte) *RequestBuilder {
	rb.bodyBytes = body
	rb.body = bytes.NewReader(body)
	return rb
}

// SetBodyString sets the request body from a string.
func (rb *RequestBuilder) SetBodyString(body string) *RequestBuilder {
	return rb.SetBodyBytes([]byte(body))
}

// SetBodyJSON marshals the value to JSON and sets it as the body.
func (rb *RequestBuilder) SetBodyJSON(v any) *RequestBuilder {
	data, err := json.Marshal(v)
	if err != nil {
		rb.err = err
		return rb
	}
	rb.SetContentType("application/json")
	return rb.SetBodyBytes(data)
}

// SetBodyXML marshals the value to XML and sets it as the body.
func (rb *RequestBuilder) SetBodyXML(v any) *RequestBuilder {
	data, err := xml.Marshal(v)
	if err != nil {
		rb.err = err
		return rb
	}
	rb.SetContentType("application/xml")
	return rb.SetBodyBytes(data)
}

// SetBodyForm sets form data as the body.
func (rb *RequestBuilder) SetBodyForm(data map[string]string) *RequestBuilder {
	form := url.Values{}
	for k, v := range data {
		form.Set(k, v)
	}
	rb.SetContentType("application/x-www-form-urlencoded")
	return rb.SetBodyString(form.Encode())
}

// Get executes a GET request.
func (rb *RequestBuilder) Get(url string) (*http.Response, error) {
	rb.method = http.MethodGet
	rb.url = url
	return rb.execute()
}

// Post executes a POST request.
func (rb *RequestBuilder) Post(url string) (*http.Response, error) {
	rb.method = http.MethodPost
	rb.url = url
	return rb.execute()
}

// Put executes a PUT request.
func (rb *RequestBuilder) Put(url string) (*http.Response, error) {
	rb.method = http.MethodPut
	rb.url = url
	return rb.execute()
}

// Patch executes a PATCH request.
func (rb *RequestBuilder) Patch(url string) (*http.Response, error) {
	rb.method = http.MethodPatch
	rb.url = url
	return rb.execute()
}

// Delete executes a DELETE request.
func (rb *RequestBuilder) Delete(url string) (*http.Response, error) {
	rb.method = http.MethodDelete
	rb.url = url
	return rb.execute()
}

// Head executes a HEAD request.
func (rb *RequestBuilder) Head(url string) (*http.Response, error) {
	rb.method = http.MethodHead
	rb.url = url
	return rb.execute()
}

// Options executes an OPTIONS request.
func (rb *RequestBuilder) Options(url string) (*http.Response, error) {
	rb.method = http.MethodOptions
	rb.url = url
	return rb.execute()
}

// Execute executes the request with the configured method.
func (rb *RequestBuilder) Execute(method, url string) (*http.Response, error) {
	rb.method = method
	rb.url = url
	return rb.execute()
}

const maxBufferBytes = 10 << 20

func bufferBody(r io.Reader) ([]byte, io.Reader, error) {
	buf, err := io.ReadAll(io.LimitReader(r, maxBufferBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(buf) > maxBufferBytes {
		return nil, io.MultiReader(bytes.NewReader(buf), r), nil
	}
	return buf, nil, nil
}

func (rb *RequestBuilder) resolveBody() (io.Reader, []byte, error) {
	if rb.body == nil {
		return nil, rb.bodyBytes, nil
	}
	if rb.bodyBytes != nil {
		return bytes.NewReader(rb.bodyBytes), rb.bodyBytes, nil
	}
	buf, stream, err := bufferBody(rb.body)
	if err != nil {
		return nil, nil, err
	}
	if stream != nil {
		return stream, nil, nil
	}
	return bytes.NewReader(buf), buf, nil
}

func (rb *RequestBuilder) execute() (*http.Response, error) {
	if rb.err != nil {
		return nil, rb.err
	}

	// Apply path parameters
	finalURL := rb.url
	for k, v := range rb.pathParams {
		finalURL = strings.ReplaceAll(finalURL, "{"+k+"}", url.PathEscape(v))
	}

	// Apply query parameters
	if len(rb.queryParams) > 0 {
		if strings.Contains(finalURL, "?") {
			finalURL += "&" + rb.queryParams.Encode()
		} else {
			finalURL += "?" + rb.queryParams.Encode()
		}
	}

	// Create body reader
	bodyReader, bodyBytes, err := rb.resolveBody()
	if err != nil {
		return nil, err
	}

	// Create request
	req, err := http.NewRequest(rb.method, finalURL, bodyReader)
	if err != nil {
		return nil, err
	}

	// Set GetBody for retry support
	if bodyBytes != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
		req.ContentLength = int64(len(bodyBytes))
	}

	// Client.Do clones the request, so sharing the builder's header map is safe.
	req.Header = rb.headers

	// Apply timeout
	ctx := rb.ctx
	if rb.timeout <= 0 {
		return rb.client.Do(ctx, req)
	}

	ctx, cancel := context.WithTimeout(ctx, rb.timeout)
	resp, err := rb.client.Do(ctx, req)
	if err != nil {
		cancel()
		return resp, err
	}
	if resp.Body == nil {
		cancel()
		return resp, nil
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// basicAuth encodes username and password for Basic authentication.
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
