package local

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

// TestEdgePresignUploadTTLBoundary checks the exact MaxPresignTTL boundary:
// the maximum is accepted, one nanosecond more is rejected.
func TestEdgePresignUploadTTLBoundary(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")

	if _, err := a.PresignUpload(t.Context(), "bkt", "k", "", storage.MaxPresignTTL); err != nil {
		t.Fatalf("PresignUpload(max ttl) error = %v, want nil", err)
	}

	if _, err := a.PresignUpload(t.Context(), "bkt", "k", "", storage.MaxPresignTTL+time.Nanosecond); err == nil {
		t.Fatal("PresignUpload(max ttl + 1ns) = nil, want error")
	}
}

// TestEdgePresignDownloadTTLBoundary checks the exact MaxPresignTTL boundary
// for downloads.
func TestEdgePresignDownloadTTLBoundary(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")

	if _, err := a.PresignDownload(t.Context(), "bkt", "k", storage.MaxPresignTTL); err != nil {
		t.Fatalf("PresignDownload(max ttl) error = %v, want nil", err)
	}

	if _, err := a.PresignDownload(t.Context(), "bkt", "k", storage.MaxPresignTTL+time.Nanosecond); err == nil {
		t.Fatal("PresignDownload(max ttl + 1ns) = nil, want error")
	}
}

// TestEdgePresignUploadInvalidKeys covers key shapes ValidKey rejects.
func TestEdgePresignUploadInvalidKeys(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")

	for _, key := range []string{"", "../x", "a/../../b", "/abs", `a\b`, "a//b", "a/", "./a"} {
		if _, err := a.PresignUpload(t.Context(), "bkt", key, "", time.Hour); !errors.Is(err, storage.ErrInvalidKey) {
			t.Errorf("PresignUpload(key %q) error = %v, want ErrInvalidKey", key, err)
		}
	}
}

// TestEdgePresignDownloadInvalidKeys covers key shapes ValidKey rejects.
func TestEdgePresignDownloadInvalidKeys(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")

	for _, key := range []string{"", "../x", "a/../../b", "/abs", `a\b`, "a//b", "a/", "./a"} {
		if _, err := a.PresignDownload(t.Context(), "bkt", key, time.Hour); !errors.Is(err, storage.ErrInvalidKey) {
			t.Errorf("PresignDownload(key %q) error = %v, want ErrInvalidKey", key, err)
		}
	}
}

// TestEdgePresignUploadReadPublicOnly returns a signed URL when only reads
// are public: public read must not widen uploads.
func TestEdgePresignUploadReadPublicOnly(t *testing.T) {
	t.Parallel()
	a := newConfigured(t, &storage.Policy{
		Read:   storage.Rule{Public: true},
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"alice"}},
	}, nil)

	got, err := a.PresignUpload(ctxWithSubject(t, "alice"), "bkt", "k", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got.URL, "sig=") {
		t.Fatalf("read-public upload should be signed: %q", got.URL)
	}
}

// TestEdgePresignDownloadWritePublicOnly returns a signed URL when only
// writes are public: public write must not widen reads.
func TestEdgePresignDownloadWritePublicOnly(t *testing.T) {
	t.Parallel()
	a := newConfigured(t, &storage.Policy{
		Read:   storage.Rule{Allow: []storage.Subject{"alice"}},
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}, nil)

	got, err := a.PresignDownload(ctxWithSubject(t, "alice"), "bkt", "k", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got.URL, "sig=") {
		t.Fatalf("write-public download should be signed: %q", got.URL)
	}
}

// TestEdgeExistsInvalidKey reports ErrInvalidKey for traversal-shaped keys.
func TestEdgeExistsInvalidKey(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")

	if _, err := a.Exists(t.Context(), "bkt", "../x"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("Exists error = %v, want ErrInvalidKey", err)
	}
}

// TestEdgeDeleteInvalidKey reports ErrInvalidKey for traversal-shaped keys.
func TestEdgeDeleteInvalidKey(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")

	if err := a.Delete(t.Context(), "bkt", "../x"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("Delete error = %v, want ErrInvalidKey", err)
	}
}

// TestEdgeDeleteStatError propagates non-NotExist filesystem errors.
func TestEdgeDeleteStatError(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")
	fileAsBucket(t, a, "bkt")

	if err := a.Delete(t.Context(), "bkt", "k"); err == nil || !strings.Contains(err.Error(), "delete") {
		t.Fatalf("Delete() = %v, want delete error", err)
	}
}

// TestEdgeMoveInvalidKeys reports validation errors for bad source and
// destination bucket/key pairs.
func TestEdgeMoveInvalidKeys(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")

	if err := a.Move(t.Context(), "bkt", "../x", "bkt", "k"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("Move(bad src key) error = %v, want ErrInvalidKey", err)
	}

	if err := a.Move(t.Context(), "bkt", "k", "", "k"); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Fatalf("Move(empty dst bucket) error = %v, want ErrInvalidBucket", err)
	}

	if err := a.Move(t.Context(), "bkt", "k", "bkt", "../x"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("Move(bad dst key) error = %v, want ErrInvalidKey", err)
	}
}

// TestEdgeMoveCrossBucketWithMeta moves an object with a sidecar across
// buckets into a nested destination path.
func TestEdgeMoveCrossBucketWithMeta(t *testing.T) {
	t.Parallel()
	a := newUnconfigured(t, "")

	srcFull := writeObject(t, a, "src", "k", "data")

	if err := os.WriteFile(srcFull+metaSuffix, []byte(`{"contentType":"text/plain"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.Move(t.Context(), "src", "k", "dst", "deep/k"); err != nil {
		t.Fatal(err)
	}

	dstFull, ok := a.resolve("dst", "deep/k")
	if !ok {
		t.Fatal("resolve failed")
	}

	body, err := os.ReadFile(dstFull)
	if err != nil || string(body) != "data" {
		t.Fatalf("dst body = %q err %v", body, err)
	}

	if _, err := os.Stat(dstFull + metaSuffix); err != nil {
		t.Fatalf("dst sidecar missing: %v", err)
	}

	if _, err := os.Stat(srcFull); !os.IsNotExist(err) {
		t.Fatalf("src still present: %v", err)
	}
}

// TestEdgeNewProdEnvAlias rejects an ephemeral secret under ZEVER_ENV=prod.
func TestEdgeNewProdEnvAlias(t *testing.T) {
	t.Setenv("ZEVER_ENV", "prod")

	_, err := New(storage.Options{LocalOptions: storage.LocalOptions{Root: t.TempDir()}})
	if err == nil || !strings.Contains(err.Error(), "required in production") {
		t.Fatalf("New() = %v, want production secret error", err)
	}
}

// TestEdgeEscapePathBoundaries covers empty keys and escapable characters.
func TestEdgeEscapePathBoundaries(t *testing.T) {
	t.Parallel()

	if got := escapePath("bkt", ""); got != "bkt/" {
		t.Errorf("escapePath empty key = %q, want bkt/", got)
	}

	if got := escapePath("bkt", "a b/c+d"); got != "bkt/a%20b/c+d" {
		t.Errorf("escapePath = %q, want bkt/a%%20b/c+d", got)
	}
}

// TestEdgeHandlerSignedGetMissing reports 404 for a signed GET on a missing
// object.
func TestEdgeHandlerSignedGetMissing(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, storage.Options{})

	pd, err := a.PresignDownload(t.Context(), "bkt", "missing.txt", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rec := doServe(t, a, http.MethodGet, signedTarget(t, pd.URL), nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing = %d, want 404", rec.Code)
	}
}

// TestEdgeHandlerSigTamper rejects URLs with a mutated signature, a rebound
// subject, or a signature minted for a different method.
func TestEdgeHandlerSigTamper(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, storage.Options{})
	ctx := t.Context()

	pd, err := a.PresignDownload(ctx, "bkt", "k", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(pd.URL)
	if err != nil {
		t.Fatal(err)
	}

	q := u.Query()
	// Replace the first hex digit with a different one: overwriting a fixed
	// prefix (e.g. "00") is a no-op for the ~1/256 of signatures that already
	// start with it, leaving a valid URL that answers 404 for the missing key.
	sig := q.Get("sig")
	first := byte('0')
	if sig[0] == '0' {
		first = '1'
	}
	q.Set("sig", string(first)+sig[1:])
	u.RawQuery = q.Encode()

	if rec := doServe(t, a, http.MethodGet, u.RequestURI(), nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("mutated sig = %d, want 403", rec.Code)
	}

	expires := time.Now().Add(time.Hour).Unix()

	rebound := url.Values{}
	rebound.Set("expires", itoa(expires))
	rebound.Set("sig", a.sign(http.MethodGet, "bkt", "k", "", expires, "alice"))
	rebound.Set("sub", "bob")

	if rec := doServe(t, a, http.MethodGet, "/bkt/k?"+rebound.Encode(), nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("rebound sub = %d, want 403", rec.Code)
	}

	pu, err := a.PresignUpload(ctx, "bkt", "k", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rec := doServe(t, a, http.MethodGet, signedTarget(t, pu.URL), nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("method mismatch = %d, want 403", rec.Code)
	}
}

// TestEdgeHandlerEmptyBodyPut stores an empty object and serves it back.
func TestEdgeHandlerEmptyBodyPut(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, storage.Options{})
	ctx := t.Context()

	pu, err := a.PresignUpload(ctx, "bkt", "empty.bin", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rec := doServe(t, a, http.MethodPut, signedTarget(t, pu.URL), strings.NewReader(""), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT empty = %d, want 204", rec.Code)
	}

	pd, err := a.PresignDownload(ctx, "bkt", "empty.bin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	rec := doServe(t, a, http.MethodGet, signedTarget(t, pd.URL), nil, nil)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("GET empty = %d body %q, want 200 empty", rec.Code, rec.Body.String())
	}
}

// TestEdgeHandlerPutSizeBoundary accepts a body of exactly maxBody bytes and
// rejects one byte more. Not parallel: it mutates the package-level maxBody.
func TestEdgeHandlerPutSizeBoundary(t *testing.T) {
	old := maxBody
	maxBody = 8

	t.Cleanup(func() { maxBody = old })

	a := newTestAdapter(t, storage.Options{})
	ctx := t.Context()

	pu, err := a.PresignUpload(ctx, "bkt", "exact.bin", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rec := doServe(t, a, http.MethodPut, signedTarget(t, pu.URL), strings.NewReader(strings.Repeat("x", 8)), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT exact = %d, want 204", rec.Code)
	}

	pu2, err := a.PresignUpload(ctx, "bkt", "over.bin", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rec := doServe(t, a, http.MethodPut, signedTarget(t, pu2.URL), strings.NewReader(strings.Repeat("x", 9)), nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("PUT over = %d, want 413", rec.Code)
	}
}

// TestEdgeDeleteThenGet404 removes an object and confirms a signed GET 404s.
func TestEdgeDeleteThenGet404(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, storage.Options{})
	ctx := t.Context()

	pu, err := a.PresignUpload(ctx, "bkt", "k", "text/plain", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rec := doServe(t, a, http.MethodPut, signedTarget(t, pu.URL), strings.NewReader("data"), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d, want 204", rec.Code)
	}

	if err = a.Delete(ctx, "bkt", "k"); err != nil {
		t.Fatal(err)
	}

	pd, err := a.PresignDownload(ctx, "bkt", "k", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rec := doServe(t, a, http.MethodGet, signedTarget(t, pd.URL), nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET after delete = %d, want 404", rec.Code)
	}
}

// TestEdgeConcurrentAccess exercises the adapter from multiple goroutines;
// core/storage documents adapters as safe for concurrent use.
func TestEdgeConcurrentAccess(t *testing.T) {
	a := newTestAdapter(t, storage.Options{})
	ctx := t.Context()

	const n = 16

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			if _, err := a.Exists(ctx, "bkt", "k"); err != nil {
				t.Errorf("Exists: %v", err)
				return
			}

			if err := a.Delete(ctx, "bkt", "k"); err != nil {
				t.Errorf("Delete: %v", err)
				return
			}

			if err := a.Move(ctx, "bkt", "src", "bkt", "dst"); err != nil && !errors.Is(err, storage.ErrNotFound) {
				t.Errorf("Move: %v", err)
			}
		}()
	}

	wg.Wait()
}

// benchFile prepares a small object under bkt/key for benchmark setup.
func benchFile(b *testing.B, a *localAdapter, key string) {
	b.Helper()

	full, ok := a.resolve("bkt", key)
	if !ok {
		b.Fatal("resolve failed")
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		b.Fatal(err)
	}

	if err := os.WriteFile(full, []byte("data"), 0o600); err != nil {
		b.Fatal(err)
	}
}
