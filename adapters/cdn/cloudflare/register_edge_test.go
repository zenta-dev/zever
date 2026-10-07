package cloudflare

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

func TestRegisterAdapter(t *testing.T) {
	t.Parallel()

	Register()
	Register()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[]}`))
	}))
	t.Cleanup(srv.Close)

	opts := validOptions()
	opts.BaseURL = srv.URL

	c, err := cdn.Open(cdn.AdapterCloudflare, opts)
	if errors.Is(err, cdn.ErrUnknownAdapter) {
		t.Fatalf("cdn.Open(%q) after Register() = %v, want adapter wired", cdn.AdapterCloudflare, err)
	}
	if err != nil {
		t.Fatalf("cdn.Open() error = %v", err)
	}
	if c != nil {
		_ = c.Close(t.Context())
	}
}
