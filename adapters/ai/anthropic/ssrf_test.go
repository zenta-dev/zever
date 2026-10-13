package anthropic

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/shared/httpclient"
)

// stubServer serves a canned completion and counts hits.
func stubServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hello world"}],"model":"claude-3","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestNew_BlocksPrivateByDefault verifies a loopback BaseURL is refused
// unless allow_private opts in, without contacting it.
func TestNew_BlocksPrivateByDefault(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubServer(t, &hits)

	a, err := New(ai.Options{APIKey: "k", Model: "claude-3", BaseURL: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	if _, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{}); !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("Generate() error = %v, want ErrPrivateAddress", err)
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("server hits = %d, want 0", got)
	}
}

// TestNew_AllowPrivateOptsIn verifies the allow_private opt-in reaches a
// loopback BaseURL.
func TestNew_AllowPrivateOptsIn(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := stubServer(t, &hits)

	a, err := New(ai.Options{APIKey: "k", Model: "claude-3", BaseURL: srv.URL, AllowInsecure: true, AllowPrivate: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	gen, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if gen.Content != "hello world" {
		t.Fatalf("Content = %q, want %q", gen.Content, "hello world")
	}

	if got := hits.Load(); got == 0 {
		t.Fatalf("server hits = %d, want >= 1", got)
	}
}

// TestNewClient_SafeByDefault verifies the shared client refuses private
// dials and verifies TLS unless explicitly opted out.
func TestNewClient_SafeByDefault(t *testing.T) {
	t.Parallel()

	c := newClient(ai.Options{})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", c.Transport)
	}

	if tr.DialContext == nil {
		t.Fatal("DialContext nil: safe dial guard not installed")
	}

	if _, err := tr.DialContext(t.Context(), "tcp", "127.0.0.1:80"); !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("DialContext() error = %v, want ErrPrivateAddress", err)
	}

	if c.CheckRedirect == nil {
		t.Fatal("CheckRedirect nil: redirects must never be followed")
	}

	if tr.TLSClientConfig == nil || tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify set without opt-in")
	}

	insecure := newClient(ai.Options{InsecureSkipVerify: true})
	itr, ok := insecure.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", insecure.Transport)
	}

	if itr.TLSClientConfig == nil || !itr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify not set with opt-in")
	}
}
