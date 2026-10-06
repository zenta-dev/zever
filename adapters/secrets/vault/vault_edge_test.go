package vault

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

// mustEdgeServer starts an httptest server running h and closes it on
// cleanup.
func mustEdgeServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return srv
}

func TestEdgeGet_MissingValueField_ErrNotFound(t *testing.T) {
	t.Parallel()

	srv := mustEdgeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"other":"x"}}}`))
	})
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})

	if _, err := s.Get(t.Context(), "k"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get(missing value field) err = %v, want ErrNotFound", err)
	}
}

func TestEdgeGet_MalformedJSON_DecodeError(t *testing.T) {
	t.Parallel()

	srv := mustEdgeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not json`))
	})
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})

	_, err := s.Get(t.Context(), "k")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("Get(malformed) err = %v, want decode error", err)
	}
}

func TestEdgeDelete_Missing_ErrNotFound(t *testing.T) {
	t.Parallel()

	srv := mustStubServer(t, "tok", "secret")
	s := mustNew(t, Options{Addr: srv.URL, Token: "tok", Mount: "secret"})

	if err := s.Delete(t.Context(), "absent"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Delete(missing) err = %v, want ErrNotFound", err)
	}
}

func TestEdgeUnexpectedStatus_Error(t *testing.T) {
	t.Parallel()

	srv := mustEdgeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})
	ctx := t.Context()

	cases := []struct {
		name string
		call func() error
	}{
		{"get", func() error { _, err := s.Get(ctx, "k"); return err }},
		{"set", func() error { return s.Set(ctx, "k", []byte("v")) }},
		{"delete", func() error { return s.Delete(ctx, "k") }},
		{"list", func() error { _, err := s.List(ctx); return err }},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.call()
			if err == nil || !strings.Contains(err.Error(), "unexpected status") {
				t.Fatalf("%s err = %v, want unexpected status", tc.name, err)
			}
		})
	}
}

func TestEdgeList_MalformedJSON_DecodeError(t *testing.T) {
	t.Parallel()

	srv := mustEdgeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`nope`))
	})
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})

	_, err := s.List(t.Context())
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("List(malformed) err = %v, want decode error", err)
	}
}

func TestEdgeList_Empty_NoError(t *testing.T) {
	t.Parallel()

	srv := mustStubServer(t, "t", "secret")
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})

	keys, err := s.List(t.Context())
	if err != nil {
		t.Fatalf("List(empty) err = %v, want nil", err)
	}
	if len(keys) != 0 {
		t.Fatalf("List(empty) = %v, want empty", keys)
	}
}

func TestEdgeSet_EmptyValue_RoundTrip(t *testing.T) {
	t.Parallel()

	srv := mustStubServer(t, "t", "secret")
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})
	ctx := t.Context()

	if err := s.Set(ctx, "empty", []byte{}); err != nil {
		t.Fatalf("Set(empty) err = %v", err)
	}

	got, err := s.Get(ctx, "empty")
	if err != nil {
		t.Fatalf("Get(empty) err = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Get(empty) len = %d, want 0", len(got))
	}
}

func TestEdgeSet_LargeValue_RoundTrip(t *testing.T) {
	t.Parallel()

	srv := mustStubServer(t, "t", "secret")
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})
	ctx := t.Context()

	value := bytes.Repeat([]byte("z"), 1<<20)
	if err := s.Set(ctx, "big", value); err != nil {
		t.Fatalf("Set(large) err = %v", err)
	}

	got, err := s.Get(ctx, "big")
	if err != nil {
		t.Fatalf("Get(large) err = %v", err)
	}
	if !bytes.Equal(got, value) {
		t.Fatalf("Get(large) round trip mismatch: got %d bytes, want %d", len(got), len(value))
	}
}

func TestEdgeName_Boundary_RoundTrip(t *testing.T) {
	t.Parallel()

	srv := mustStubServer(t, "t", "secret")
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})
	ctx := t.Context()

	name := strings.Repeat("a", 512)
	if err := s.Set(ctx, name, []byte("v")); err != nil {
		t.Fatalf("Set(long name) err = %v", err)
	}

	got, err := s.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get(long name) err = %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("Get(long name) = %q, want %q", got, "v")
	}
}

func TestEdgeTransportError_Propagates(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := srv.URL
	srv.Close()

	s := mustNew(t, Options{Addr: addr, Token: "t", Mount: "secret"})
	ctx := t.Context()

	cases := []struct {
		name string
		call func() error
	}{
		{"get", func() error { _, err := s.Get(ctx, "k"); return err }},
		{"set", func() error { return s.Set(ctx, "k", []byte("v")) }},
		{"delete", func() error { return s.Delete(ctx, "k") }},
		{"list", func() error { _, err := s.List(ctx); return err }},
	}

	for _, tc := range cases {
		if err := tc.call(); err == nil {
			t.Errorf("%s over closed server err = nil, want transport error", tc.name)
		}
	}
}

func TestEdgeNew_AddrTrailingSlash_Trimmed(t *testing.T) {
	t.Parallel()

	srv := mustStubServer(t, "t", "secret")
	s := mustNew(t, Options{Addr: srv.URL + "/", Token: "t", Mount: "secret"})

	if err := s.Set(t.Context(), "k", []byte("v")); err != nil {
		t.Fatalf("Set with trailing-slash addr err = %v", err)
	}
}

// TestEdgeNamespaceHeader_Sent verifies a configured namespace is forwarded on
// every request.
func TestEdgeNamespaceHeader_Sent(t *testing.T) {
	t.Parallel()

	got := make(chan string, 1)

	srv := mustEdgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Vault-Namespace")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"value":"aGk="}}}`))
	})
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret", Namespace: "team-a"})

	if _, err := s.Get(t.Context(), "k"); err != nil {
		t.Fatalf("Get() err = %v", err)
	}

	if ns := <-got; ns != "team-a" {
		t.Fatalf("X-Vault-Namespace = %q, want %q", ns, "team-a")
	}
}

// TestEdgeList_NotFound_NoError covers the 404 metadata branch: an unmounted
// KV path lists as empty rather than failing.
func TestEdgeList_NotFound_NoError(t *testing.T) {
	t.Parallel()

	srv := mustEdgeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})

	keys, err := s.List(t.Context())
	if err != nil {
		t.Fatalf("List(404) err = %v, want nil", err)
	}

	if len(keys) != 0 {
		t.Fatalf("List(404) = %v, want empty", keys)
	}
}

func TestEdgeConcurrent_Access(t *testing.T) {
	t.Parallel()

	srv := mustStubServer(t, "t", "secret")
	s := mustNew(t, Options{Addr: srv.URL, Token: "t", Mount: "secret"})
	ctx := t.Context()

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			name := fmt.Sprintf("key-%d", i)
			if err := s.Set(ctx, name, []byte("v")); err != nil {
				t.Errorf("goroutine %d: Set err = %v", i, err)
				return
			}
			if _, err := s.Get(ctx, name); err != nil {
				t.Errorf("goroutine %d: Get err = %v", i, err)
			}
			if _, err := s.List(ctx); err != nil {
				t.Errorf("goroutine %d: List err = %v", i, err)
			}
			if err := s.Delete(ctx, name); err != nil {
				t.Errorf("goroutine %d: Delete err = %v", i, err)
			}
		}(i)
	}

	wg.Wait()
}
