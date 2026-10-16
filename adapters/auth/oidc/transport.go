package oidc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/zenta-dev/zever/shared/httpclient"
)

const (
	// maxResponseBytes caps a discovery document or JWKS body. go-oidc reads
	// both with an unbounded io.ReadAll, so a hostile or compromised issuer
	// could otherwise stream memory exhaustion into the process.
	maxResponseBytes = 1 << 20

	// minRefetchInterval throttles repeat GETs of one URL. go-oidc refetches
	// the JWKS on every token whose kid is not cached, so a stream of forged
	// tokens with random kids would otherwise turn into a stream of requests
	// to the IdP. Within the interval the previous successful response is
	// replayed, which also means a freshly rotated key can take up to this
	// long to become visible.
	minRefetchInterval = 10 * time.Second
)

// guardedTransport bounds response bodies and throttles repeat GETs. It is
// safe for concurrent use.
type guardedTransport struct {
	next  http.RoundTripper
	limit int64
	min   time.Duration
	now   func() time.Time

	mu   sync.Mutex
	seen map[string]cachedResponse
}

type cachedResponse struct {
	at     time.Time
	status int
	header http.Header
	body   []byte
}

func newGuardedTransport(next http.RoundTripper) *guardedTransport {
	if next == nil {
		next = http.DefaultTransport
	}

	return &guardedTransport{
		next:  next,
		limit: maxResponseBytes,
		min:   minRefetchInterval,
		now:   time.Now,
		seen:  make(map[string]cachedResponse),
	}
}

func (t *guardedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key := req.URL.String()

	if req.Method == http.MethodGet {
		t.mu.Lock()
		c, ok := t.seen[key]
		t.mu.Unlock()

		if ok && t.now().Sub(c.at) < t.min {
			return c.response(req), nil
		}
	}

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := httpclient.ReadLimited(req.Context(), resp.Body, t.limit)
	if err != nil {
		return nil, fmt.Errorf("oidc: read %s response: %w", req.URL.Redacted(), err)
	}

	c := cachedResponse{at: t.now(), status: resp.StatusCode, header: resp.Header.Clone(), body: body}

	if req.Method == http.MethodGet && resp.StatusCode == http.StatusOK {
		t.mu.Lock()
		t.seen[key] = c
		t.mu.Unlock()
	}

	return c.response(req), nil
}

func (c cachedResponse) response(req *http.Request) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", c.status, http.StatusText(c.status)),
		StatusCode:    c.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        c.header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(c.body)),
		ContentLength: int64(len(c.body)),
		Request:       req,
	}
}
