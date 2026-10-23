package vault

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

// mustTokenFile writes a token file with 0600 then chmods it to perm,
// so the on-disk mode is exact regardless of umask.
func mustTokenFile(t *testing.T, perm os.FileMode) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "vault-token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	return path
}

// TestValidate_TokenFilePerms pins the P2-11b fix: a token file readable
// beyond the owner (any group/other bit, i.e. broader than 0600) fails
// closed unless the operator explicitly opts out.
func TestValidate_TokenFilePerms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		perm    os.FileMode
		optOut  bool
		wantErr bool
	}{
		{"owner only accepted", 0o600, false, false},
		{"owner read-only accepted", 0o400, false, false},
		{"group readable rejected", 0o640, false, true},
		{"world readable rejected", 0o644, false, true},
		{"opt-out permits broad perms", 0o644, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := mustTokenFile(t, tc.perm)
			opts := Options{
				Addr:          "http://127.0.0.1:8200",
				AllowInsecure: true, Mount: "secret",
				TokenFile:                   path,
				AllowInsecureTokenFilePerms: tc.optOut,
			}
			err := opts.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrInsecureTokenFilePerms) {
					t.Fatalf("Validate() = %v, want ErrInsecureTokenFilePerms", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestResolveToken_InsecurePerms_FailsClosed proves the permission check
// is enforced at token resolution too, not only in Validate, so a direct
// Go-API caller cannot bypass it by skipping validation.
func TestResolveToken_InsecurePerms_FailsClosed(t *testing.T) {
	t.Parallel()

	path := mustTokenFile(t, 0o644)
	if _, err := (Options{TokenFile: path}).resolveToken(); !errors.Is(err, ErrInsecureTokenFilePerms) {
		t.Fatalf("resolveToken() = %v, want ErrInsecureTokenFilePerms", err)
	}
	got, err := (Options{TokenFile: path, AllowInsecureTokenFilePerms: true}).resolveToken()
	if err != nil || got != "file-token" {
		t.Fatalf("resolveToken() = (%q, %v), want (file-token, nil)", got, err)
	}
}

// TestNew_TokenFilePerms_FailsClosed proves New enforces the same rule
// (Validate is not optional at construction).
func TestNew_TokenFilePerms_FailsClosed(t *testing.T) {
	t.Parallel()

	path := mustTokenFile(t, 0o644)
	opts := Options{Addr: "http://127.0.0.1:1", AllowInsecure: true, Mount: "secret", TokenFile: path}
	if _, err := New(opts); !errors.Is(err, ErrInsecureTokenFilePerms) {
		t.Fatalf("New() = %v, want ErrInsecureTokenFilePerms", err)
	}
}

// TestTokenFile_OptOut_RoundTrip proves the opt-out still yields a working
// adapter against a stub server.
func TestTokenFile_OptOut_RoundTrip(t *testing.T) {
	srv := mustStubServer(t, "file-token", "secret")
	path := mustTokenFile(t, 0o644)
	s := mustNew(t, Options{
		Addr: srv.URL, AllowInsecure: true, Mount: "secret",
		TokenFile: path, AllowInsecureTokenFilePerms: true,
	})
	if err := s.Set(t.Context(), "k", []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(t.Context(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("got %q, want %q", got, "v")
	}
}

// TestEscapeMount pins the per-segment escaping: separators survive,
// reserved characters are encoded.
func TestEscapeMount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"secret", "secret"},
		{"team/secret", "team/secret"},
		{"/secret/", "secret"},
		{"a?b", "a%3Fb"},
		{"a#b", "a%23b"},
		{"a%b", "a%25b"},
		{"a b", "a%20b"},
	}
	for _, tc := range cases {
		if got := escapeMount(tc.in); got != tc.want {
			t.Errorf("escapeMount(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestValidate_MountSegments pins the P2-11c validation: empty and dot
// segments are rejected, nested mounts and escapable characters pass.
func TestValidate_MountSegments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		mount   string
		wantErr string
	}{
		{"secret", ""},
		{"team/secret", ""},
		{"a?b", ""},
		{"a//b", "empty"},
		{"a/./b", "dot"},
		{"a/../b", "dot"},
	}
	for _, tc := range cases {
		t.Run(tc.mount, func(t *testing.T) {
			t.Parallel()

			err := (Options{Addr: "http://127.0.0.1:8200", AllowInsecure: true, Token: "t", Mount: tc.mount}).Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate(%q) = %v, want nil", tc.mount, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate(%q) = %v, want error containing %q", tc.mount, err, tc.wantErr)
			}
		})
	}
}

// TestMount_EscapedOnWire proves the mount cannot shift URL path segments:
// with a `?` in the mount, every request must still arrive under the
// escaped mount prefix instead of leaking into the query string.
func TestMount_EscapedOnWire(t *testing.T) {
	const mount = "odd?mount"
	const escaped = "/v1/odd%3Fmount/"

	var mu sync.Mutex
	store := map[string]string{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.EscapedPath(), escaped) {
			t.Errorf("request path %q escapes mount prefix %q", r.URL.EscapedPath(), escaped)
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.EscapedPath(), "/metadata/") && r.URL.Query().Get("list") == "true":
			keys := make([]string, 0, len(store))
			for k := range store {
				keys = append(keys, k)
			}
			_ = codec.MarshalLine(w, map[string]any{"data": map[string]any{"keys": keys}})
		case strings.Contains(r.URL.EscapedPath(), "/data/"):
			name := r.URL.EscapedPath()[len(escaped+"data/"):]
			switch r.Method {
			case http.MethodPut:
				var body struct {
					Data map[string]string `json:"data"`
				}
				if err := codec.UnmarshalRead(r.Body, &body); err != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				store[name] = body.Data["value"]
				w.WriteHeader(http.StatusNoContent)
			case http.MethodGet:
				val, ok := store[name]
				if !ok {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				_ = codec.MarshalLine(w, map[string]any{
					"data": map[string]any{"data": map[string]string{"value": val}},
				})
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case strings.Contains(r.URL.EscapedPath(), "/metadata/"):
			name := r.URL.EscapedPath()[len(escaped+"metadata/"):]
			if _, ok := store[name]; !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			delete(store, name)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "bad path", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	s := mustNew(t, Options{Addr: srv.URL, AllowInsecure: true, Token: "t", Mount: mount})
	ctx := t.Context()

	if err := s.Set(ctx, "k", []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("got %q, want %q", got, "v")
	}
	keys, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 || keys[0] != "k" {
		t.Fatalf("List = %v, want [k]", keys)
	}
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// TestMount_Nested_RoundTrip guards the legitimate nested-mount shape:
// slashes still separate segments after escaping.
func TestMount_Nested_RoundTrip(t *testing.T) {
	srv := mustStubServer(t, "t", "team/secret")
	s := mustNew(t, Options{Addr: srv.URL, AllowInsecure: true, Token: "t", Mount: "team/secret"})
	if err := s.Set(t.Context(), "k", []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(t.Context(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("got %q, want %q", got, "v")
	}
}
