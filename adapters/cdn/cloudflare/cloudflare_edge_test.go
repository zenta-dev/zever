package cloudflare

import (
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

func TestEdgeEmptyPurgeRequest(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = c.Purge(t.Context(), cdn.PurgeRequest{})
	if err == nil {
		t.Fatal("Purge(empty) = nil, want error")
	}
}

func TestEdgeAPIReturnsError(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, `{"success":false,"errors":[{"code":1000,"message":"invalid zone"}],"messages":[]}`, 403)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = c.Purge(t.Context(), cdn.PurgeRequest{URLs: []string{"https://example.com"}})
	if err == nil {
		t.Fatal("Purge() = nil, want error")
	}
}

func TestEdgeAPIReturns500(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, `{"success":false,"errors":[],"messages":[]}`, 500)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = c.Purge(t.Context(), cdn.PurgeRequest{All: true})
	if err == nil {
		t.Fatal("Purge() = nil, want error")
	}
}

func TestEdgeClosedAdapter(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = c.Close(t.Context())
	err = c.Purge(t.Context(), cdn.PurgeRequest{URLs: []string{"https://example.com"}})
	if err == nil {
		t.Fatal("Purge() after Close = nil, want error")
	}
}

func TestEdgeConcurrentPurge(t *testing.T) {
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Purge(t.Context(), cdn.PurgeRequest{URLs: []string{"https://example.com/style.css"}})
		}()
	}
	wg.Wait()
}
