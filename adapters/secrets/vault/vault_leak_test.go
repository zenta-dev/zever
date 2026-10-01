package vault

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mustLeakServer(t *testing.T, token string) (*httptest.Server, Options) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv, Options{Addr: srv.URL, Token: token, Mount: "secret"}
}

// TestErrors_NeverLeakTokenOrValue ensures Vault failure paths include only
// the secret name and status, never auth material or secret bytes.
func TestErrors_NeverLeakTokenOrValue(t *testing.T) {
	t.Parallel()
	const token = "s.leak-check-token-abc123"
	const value = "super-secret-value-xyz"
	srv, opts := mustLeakServer(t, token)
	_ = srv
	d, err := New(opts)
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	ctx := t.Context()
	for name, fn := range map[string]func() error{
		"get":    func() error { _, err := d.Get(ctx, "mykey"); return err },
		"set":    func() error { return d.Set(ctx, "mykey", []byte(value)) },
		"delete": func() error { return d.Delete(ctx, "mykey") },
		"list":   func() error { _, err := d.List(ctx); return err },
	} {
		if err := fn(); err == nil {
			t.Fatalf("%s: expected error, got nil", name)
		} else if s := err.Error(); strings.Contains(s, token) || strings.Contains(s, value) {
			t.Fatalf("%s: error leaks secret material: %q", name, s)
		}
	}
}
