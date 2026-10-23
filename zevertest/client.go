package zevertest

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

// Client drives an http.Handler in-process: no port, no network. Requests run
// through ServeHTTP on the test goroutine, so handlers see exactly what they
// would behind a real listener minus TLS and connection handling.
//
// A Client is immutable once built: WithHeader and WithBearer return copies,
// so a shared base client can fan out into per-user clients safely.
type Client struct {
	tb     testing.TB
	h      http.Handler
	header http.Header
}

// NewClient returns a Client for h, typically a router.Router built from the
// test container (c.Router()) with routes registered.
func NewClient(tb testing.TB, h http.Handler) *Client {
	tb.Helper()

	return &Client{tb: tb, h: h, header: http.Header{}}
}

// WithHeader returns a copy that sends key: value on every request.
func (c *Client) WithHeader(key, value string) *Client {
	cp := *c
	cp.header = c.header.Clone()
	cp.header.Set(key, value)

	return &cp
}

// WithBearer returns a copy that sends "Authorization: Bearer <token>".
func (c *Client) WithBearer(token string) *Client {
	return c.WithHeader("Authorization", "Bearer "+token)
}

// Response is the recorded result of one request.
type Response struct {
	tb testing.TB
	// Status is the HTTP status code.
	Status int
	// Header holds the response headers.
	Header http.Header
	// Body is the full response body.
	Body []byte
}

// Do sends one request. body may be nil, []byte, string, io.Reader, or any
// value json.Marshal accepts (sent as application/json). The test fails
// immediately on a marshal or request-construction error.
func (c *Client) Do(method, path string, body any) *Response {
	c.tb.Helper()

	var (
		reader      io.Reader
		contentType string
	)

	switch b := body.(type) {
	case nil:
	case []byte:
		reader = bytes.NewReader(b)
	case string:
		reader = strings.NewReader(b)
	case io.Reader:
		reader = b
	default:
		raw, err := codec.Marshal(b)
		if err != nil {
			c.tb.Fatalf("zevertest: marshal %s %s body: %v", method, path, err)
		}

		reader = bytes.NewReader(raw)
		contentType = "application/json"
	}

	req := httptest.NewRequestWithContext(c.tb.Context(), method, path, reader)

	for k, vs := range c.header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}

	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		c.tb.Fatalf("zevertest: read %s %s response: %v", method, path, err)
	}

	return &Response{tb: c.tb, Status: res.StatusCode, Header: res.Header, Body: raw}
}

// Get sends a GET request.
func (c *Client) Get(path string) *Response { c.tb.Helper(); return c.Do(http.MethodGet, path, nil) }

// Post sends a POST request with body (see Do).
func (c *Client) Post(path string, body any) *Response {
	c.tb.Helper()

	return c.Do(http.MethodPost, path, body)
}

// Put sends a PUT request with body (see Do).
func (c *Client) Put(path string, body any) *Response {
	c.tb.Helper()

	return c.Do(http.MethodPut, path, body)
}

// Patch sends a PATCH request with body (see Do).
func (c *Client) Patch(path string, body any) *Response {
	c.tb.Helper()

	return c.Do(http.MethodPatch, path, body)
}

// Delete sends a DELETE request.
func (c *Client) Delete(path string) *Response {
	c.tb.Helper()

	return c.Do(http.MethodDelete, path, nil)
}

// Text returns the body as a string.
func (r *Response) Text() string { return string(r.Body) }

// JSON decodes the body into v, failing the test on malformed JSON.
func (r *Response) JSON(v any) {
	r.tb.Helper()

	if err := codec.Unmarshal(r.Body, v); err != nil {
		r.tb.Fatalf("zevertest: decode response body %q: %v", r.Body, err)
	}
}

// RequireStatus fails the test, showing the body, unless the status is want.
// It returns r so calls chain: c.Get("/x").RequireStatus(200).JSON(&v).
func (r *Response) RequireStatus(want int) *Response {
	r.tb.Helper()

	if r.Status != want {
		r.tb.Fatalf("zevertest: status = %d, want %d; body: %s", r.Status, want, r.Body)
	}

	return r
}
