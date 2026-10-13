package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/shared/httpclient"
)

// stubTranslateServer serves one translation and counts hits.
func stubTranslateServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"translations":{"hello":"Hello"}}`))
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestNew_BlocksPrivateByDefault verifies a loopback endpoint is refused
// unless allow_private opts in, without contacting it.
func TestNew_BlocksPrivateByDefault(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubTranslateServer(t, &hits)

	ad, err := New(i18n.Options{Remote: i18n.RemoteOptions{Endpoint: srv.URL, AllowInsecure: true}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = ad.Close() }()

	if _, err := ad.Translate(t.Context(), "en", "hello", nil); !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("Translate() error = %v, want ErrPrivateAddress", err)
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("server hits = %d, want 0", got)
	}
}

// TestNew_AllowPrivateOptsIn verifies the allow_private opt-in reaches a
// loopback catalog sidecar.
func TestNew_AllowPrivateOptsIn(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubTranslateServer(t, &hits)

	ad, err := New(i18n.Options{Remote: i18n.RemoteOptions{Endpoint: srv.URL, AllowInsecure: true, AllowPrivate: true}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = ad.Close() }()

	got, err := ad.Translate(t.Context(), "en", "hello", nil)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}

	if got != "Hello" {
		t.Fatalf("Translate() = %q, want %q", got, "Hello")
	}

	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}
