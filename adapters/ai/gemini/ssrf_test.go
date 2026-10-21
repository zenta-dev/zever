package gemini

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	// The guard refuses before the dial, so the refusal keeps the
	// httpclient sentinel in its chain: assert on the typed error, not on
	// the message text.
	_, err = a.Generate(t.Context(), "m", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("Generate() error = %v, want private-address refusal", err)
	}
	if errors.Is(err, ai.ErrRateLimited) {
		t.Fatalf("Generate() error = %v, want private-address refusal, not a rate limit", err)
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("server hits = %d, want 0", got)
	}
}

// TestMapError_PrivateAddressPortIsNotAStatus pins the classifier against
// the confusing case: the SDK reports a transport failure as *url.Error,
// whose text carries the request URL, and the URL carries an ephemeral
// port. A port that reads like an HTTP status (42901 reads as 429) must
// not turn the private-address refusal into a rate limit.
func TestMapError_PrivateAddressPortIsNotAStatus(t *testing.T) {
	t.Parallel()

	// The real guard error: the dial is refused before it happens, so no
	// traffic leaves the machine.
	dial := httpclient.SafeDialContext(false)
	_, guardErr := dial(t.Context(), "tcp", "127.0.0.1:42901")
	if !errors.Is(guardErr, httpclient.ErrPrivateAddress) {
		t.Fatalf("guard error = %v, want ErrPrivateAddress", guardErr)
	}

	// The shape net/http reports to the SDK, and the SDK wraps.
	transport := &url.Error{
		Op:  "Post",
		URL: "https://127.0.0.1:42901/v1beta/models/gemini-2.5-flash:generateContent",
		Err: guardErr,
	}

	mapped := mapError(transport)
	if errors.Is(mapped, ai.ErrRateLimited) {
		t.Fatalf("mapError() = %v, want the private-address refusal, not a rate limit", mapped)
	}
	if !errors.Is(mapped, httpclient.ErrPrivateAddress) {
		t.Fatalf("mapError() = %v, want ErrPrivateAddress", mapped)
	}
}

// TestGenerate_PrivateAddressPortIsNotAStatus is the end-to-end twin of
// TestMapError_PrivateAddressPortIsNotAStatus: the refusal travels out of
// the adapter as itself. The guard refuses before the dial, so port 42901
// needs no listener.
func TestGenerate_PrivateAddressPortIsNotAStatus(t *testing.T) {
	t.Parallel()

	const baseURL = "https://127.0.0.1:42901"

	a, err := New(ai.Options{APIKey: "k", BaseURL: baseURL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(t.Context(), "m", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil {
		t.Fatal("Generate() error = nil, want the private-address refusal")
	}
	if errors.Is(err, ai.ErrRateLimited) {
		t.Fatalf("Generate() error = %v, want the private-address refusal, not a rate limit", err)
	}
	if !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("Generate() error = %v, want ErrPrivateAddress", err)
	}
	if !strings.Contains(err.Error(), "42901") {
		t.Fatalf("Generate() error = %v, want the refused address in the message", err)
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
