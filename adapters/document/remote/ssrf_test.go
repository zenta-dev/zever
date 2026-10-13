package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/shared/httpclient"
)

// stubServer serves a fixed PDF body and counts hits.
func stubServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 stub"))
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestNew_BlocksPrivateByDefault verifies the remote renderer refuses a
// loopback endpoint unless allow_private opts in, without contacting it.
func TestNew_BlocksPrivateByDefault(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubServer(t, &hits)

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("Render() error = %v, want ErrPrivateAddress", err)
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("server hits = %d, want 0", got)
	}
}

// TestNew_AllowPrivateOptsIn verifies the allow_private opt-in reaches a
// loopback rendering sidecar.
func TestNew_AllowPrivateOptsIn(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubServer(t, &hits)

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true, AllowPrivate: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = doc.Close() }()

	got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(got) != "%PDF-1.4 stub" {
		t.Fatalf("Render() = %q, want stub body", got)
	}

	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}
