package vault

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

type stubVault struct {
	t     *testing.T
	mu    sync.Mutex
	store map[string]string
	token string
	mount string
}

func (s *stubVault) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/"+s.mount+"/data/", s.handleData)
	mux.HandleFunc("/v1/"+s.mount+"/metadata/", s.handleMetadata)
	return mux
}

func (s *stubVault) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("X-Vault-Token") != s.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (s *stubVault) handleData(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(w, r) {
		return
	}
	prefix := "/v1/" + s.mount + "/data/"
	name := strings.TrimPrefix(r.URL.Path, prefix)
	switch r.Method {
	case http.MethodPut, http.MethodPost:
		var body struct {
			Data map[string]string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.store[name] = body.Data["value"]
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		s.mu.Lock()
		val, ok := s.store[name]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data":     map[string]string{"value": val},
				"metadata": map[string]any{},
			},
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *stubVault) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(w, r) {
		return
	}
	prefix := "/v1/" + s.mount + "/metadata/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if r.Method == http.MethodGet && (rest == "" || rest == "/") && r.URL.Query().Get("list") == "true" {
		s.mu.Lock()
		keys := make([]string, 0, len(s.store))
		for k := range s.store {
			keys = append(keys, k)
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"keys": keys},
		})
		return
	}
	if r.Method == http.MethodDelete {
		s.mu.Lock()
		_, ok := s.store[rest]
		if ok {
			delete(s.store, rest)
		}
		s.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func mustStubServer(t *testing.T, token, mount string) *httptest.Server {
	t.Helper()
	stub := &stubVault{t: t, store: map[string]string{}, token: token, mount: mount}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	return srv
}

func mustNew(t *testing.T, opts Options) secrets.Secrets {
	t.Helper()
	s, err := New(opts)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return s
}

func TestRoundTrip_GetSetDeleteList(t *testing.T) {
	srv := mustStubServer(t, "test-token", "secret")
	s := mustNew(t, Options{Addr: srv.URL, Token: "test-token", Mount: "secret"})
	ctx := t.Context()

	if err := s.Set(ctx, "alpha", []byte("hello")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, err := s.Get(ctx, "alpha")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "hello" {
		t.Fatalf("got %q, want %q", val, "hello")
	}
	if err = s.Set(ctx, "beta", []byte("world")); err != nil {
		t.Fatalf("Set beta: %v", err)
	}
	keys, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := map[string]bool{}
	for _, k := range keys {
		found[k] = true
	}
	if !found["alpha"] || !found["beta"] {
		t.Fatalf("want alpha and beta, got %v", keys)
	}
	if err := s.Delete(ctx, "alpha"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "alpha"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get deleted err = %v, want ErrNotFound", err)
	}
}

func TestGet_RawValue_ErrInvalidBase64(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"data": map[string]string{"value": "!!!not-base64!!!"}},
		})
	}))
	t.Cleanup(srv.Close)

	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})
	if _, err := s.Get(t.Context(), "raw"); !errors.Is(err, ErrInvalidBase64) {
		t.Fatalf("err = %v, want ErrInvalidBase64", err)
	}
}

func TestGet_Missing_ErrNotFound(t *testing.T) {
	srv := mustStubServer(t, "test-token", "secret")
	s := mustNew(t, Options{Addr: srv.URL, Token: "test-token", Mount: "secret"})
	if _, err := s.Get(t.Context(), "does-not-exist"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestTokenFile_Read(t *testing.T) {
	srv := mustStubServer(t, "file-token", "secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := mustNew(t, Options{Addr: srv.URL, TokenFile: path, Mount: "secret"})
	if err := s.Set(t.Context(), "k", []byte("v")); err != nil {
		t.Fatalf("Set with token file: %v", err)
	}
	val, err := s.Get(t.Context(), "k")
	if err != nil {
		t.Fatalf("Get with token file: %v", err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q, want %q", val, "v")
	}
}

func TestInvalidName_Rejected(t *testing.T) {
	t.Parallel()
	s := mustNew(t, Options{Addr: "http://127.0.0.1:1", Token: "t", Mount: "secret"})
	ctx := t.Context()
	for _, name := range []string{"", "a/b", "a..b", "a b", "a!b"} {
		if _, err := s.Get(ctx, name); !errors.Is(err, secrets.ErrInvalidKey) {
			t.Errorf("Get(%q) err = %v, want ErrInvalidKey", name, err)
		}
		if err := s.Set(ctx, name, []byte("v")); !errors.Is(err, secrets.ErrInvalidKey) {
			t.Errorf("Set(%q) err = %v, want ErrInvalidKey", name, err)
		}
		if err := s.Delete(ctx, name); !errors.Is(err, secrets.ErrInvalidKey) {
			t.Errorf("Delete(%q) err = %v, want ErrInvalidKey", name, err)
		}
	}
}

func TestNew_Validate_FailsClosed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts Options
	}{
		{"empty", Options{}},
		{"no addr", Options{Token: "t", Mount: "secret"}},
		{"bad addr", Options{Addr: "://bad", Token: "t", Mount: "secret"}},
		{"no mount", Options{Addr: "http://127.0.0.1:8200", Token: "t"}},
		{"no token", Options{Addr: "http://127.0.0.1:8200", Mount: "secret"}},
		{"bad token file", Options{Addr: "http://127.0.0.1:8200", Mount: "secret", TokenFile: "/does/not/exist"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(tc.opts); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestClose_NilError(t *testing.T) {
	t.Parallel()
	s := mustNew(t, Options{Addr: "http://127.0.0.1:1", Token: "t", Mount: "secret"})
	if err := s.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
