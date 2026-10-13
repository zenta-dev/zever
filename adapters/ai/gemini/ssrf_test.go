package gemini

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/shared/httpclient"
)

// stubServer serves a canned completion and counts hits.
func stubServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"}}]}`))
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

	a, err := New(ai.Options{APIKey: "k", BaseURL: srv.URL, AllowInsecure: true, InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	// The genai SDK remaps transport errors (its status-code sniffing can
	// misread an ephemeral port as a status), so assert on the guard
	// message rather than the sentinel: the private dial never happens.
	_, err = a.Generate(t.Context(), "m", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), httpclient.ErrPrivateAddress.Error()) {
		t.Fatalf("Generate() error = %v, want private-address refusal", err)
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

	a, err := New(ai.Options{APIKey: "k", BaseURL: srv.URL, AllowInsecure: true, AllowPrivate: true, InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	gen, err := a.Generate(t.Context(), "m", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if gen.Content != "ok" {
		t.Fatalf("Content = %q, want %q", gen.Content, "ok")
	}

	if got := hits.Load(); got == 0 {
		t.Fatalf("server hits = %d, want >= 1", got)
	}
}
