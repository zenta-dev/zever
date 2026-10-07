package cloudflare

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

func newTestServer(t *testing.T, body string, code int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func validOptions() cdn.Options {
	return cdn.Options{
		APIToken: "test-token",
		ZoneID:   "test-zone-id",
	}
}

func TestNewValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		opts cdn.Options
	}{
		{name: "missing api token", opts: cdn.Options{ZoneID: "zone"}},
		{name: "missing zone id", opts: cdn.Options{APIToken: "token"}},
		{name: "bad base url", opts: cdn.Options{APIToken: "token", ZoneID: "zone", BaseURL: "://bad"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts)
			if err == nil {
				t.Fatal("New() = nil, want error")
			}
		})
	}
}

func TestNewValidOptions(t *testing.T) {
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	a, ok := c.(*cloudflareAdapter)
	if !ok {
		t.Fatalf("New() type = %T, want *cloudflareAdapter", c)
	}
	if a.zoneID != "test-zone-id" {
		t.Errorf("zoneID = %q, want %q", a.zoneID, "test-zone-id")
	}
}

func TestPurgeByURL(t *testing.T) {
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = c.Purge(t.Context(), cdn.PurgeRequest{URLs: []string{"https://example.com/style.css"}})
	if err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
}

func TestPurgeByTag(t *testing.T) {
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = c.Purge(t.Context(), cdn.PurgeRequest{Tags: []string{"tag1"}})
	if err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
}

func TestPurgeAll(t *testing.T) {
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = c.Purge(t.Context(), cdn.PurgeRequest{All: true})
	if err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
}

func TestName(t *testing.T) {
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := c.Name(); got != "cloudflare" {
		t.Errorf("Name() = %q, want %q", got, "cloudflare")
	}
}

func TestClose(t *testing.T) {
	srv := newTestServer(t, `{"success":true,"errors":[],"messages":[]}`, 200)
	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Close(t.Context()); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
