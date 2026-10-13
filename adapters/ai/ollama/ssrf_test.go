package ollama

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/shared/httpclient"
)

// stubServer serves a canned chat completion and counts hits.
func stubServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"hi"},"done":true,"eval_count":1,"prompt_eval_count":2}`))
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestNewWithOptions_BlocksPrivateByDefault verifies the loopback default
// address is refused unless allow_private opts in, without contacting it.
func TestNewWithOptions_BlocksPrivateByDefault(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubServer(t, &hits)

	a, err := NewWithOptions(Options{Addr: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("NewWithOptions() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	if _, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{}); !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("Generate() error = %v, want ErrPrivateAddress", err)
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("server hits = %d, want 0", got)
	}
}

// TestNewWithOptions_AllowPrivateOptsIn verifies the allow_private opt-in
// reaches a local server.
func TestNewWithOptions_AllowPrivateOptsIn(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubServer(t, &hits)

	a, err := NewWithOptions(Options{Addr: srv.URL, Model: "m", AllowPrivate: true})
	if err != nil {
		t.Fatalf("NewWithOptions() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	gen, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if gen.Content != "hi" {
		t.Fatalf("Content = %q, want %q", gen.Content, "hi")
	}

	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

// TestNew_MapsPrivateOptions verifies ai.Options allow_private and
// insecure_skip_verify flow through to the adapter client.
func TestNew_MapsPrivateOptions(t *testing.T) {
	t.Parallel()

	a, err := New(ai.Options{Model: "m", AllowPrivate: true, InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	ad, ok := a.(*adapter)
	if !ok {
		t.Fatalf("New() = %T, want *adapter", a)
	}

	tr, ok := ad.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", ad.client.Transport)
	}

	if tr.DialContext == nil {
		t.Fatal("DialContext nil: safe dial guard not installed")
	}

	// With AllowPrivate the guard must pass: the probe either dials (refused
	// fast when nothing listens) or connects; it must not be refused as
	// a private address.
	conn, err := tr.DialContext(t.Context(), "tcp", "127.0.0.1:80")
	if errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatal("DialContext() refused private address despite AllowPrivate=true")
	}
	if conn != nil {
		_ = conn.Close()
	}

	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify not set with opt-in")
	}
}
