package oidc

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/shared/httpclient"
)

func TestGuardedTransportRejectsOversizeBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 64))
	}))
	t.Cleanup(srv.Close)

	gt := newGuardedTransport(nil)
	gt.limit = 16

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := gt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if !errors.Is(err, httpclient.ErrTooLarge) {
		t.Fatalf("RoundTrip err = %v, want ErrTooLarge", err)
	}
}

func TestGuardedTransportThrottlesRefetch(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, "body")
	}))
	t.Cleanup(srv.Close)

	now := time.Unix(1000, 0)
	gt := newGuardedTransport(nil)
	gt.now = func() time.Time { return now }

	get := func() string {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := gt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = resp.Body.Close() }()

		b, _ := io.ReadAll(resp.Body)

		return string(b)
	}

	for range 5 {
		if got := get(); got != "body" {
			t.Fatalf("body = %q", got)
		}
	}

	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits within interval = %d, want 1", got)
	}

	now = now.Add(gt.min + time.Second)
	get()

	if got := hits.Load(); got != 2 {
		t.Fatalf("upstream hits after interval = %d, want 2", got)
	}
}
