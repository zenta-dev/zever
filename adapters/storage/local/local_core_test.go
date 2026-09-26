package local

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

const testSecret = "test-secret-0123456789abcdef"

func mustAdapter(t *testing.T, s storage.Storage) *localAdapter {
	t.Helper()

	a, ok := s.(*localAdapter)
	if !ok {
		t.Fatal("New did not return *localAdapter")
	}

	return a
}

func newUnconfigured(t *testing.T, urlBase string) *localAdapter {
	t.Helper()

	s, err := New(storage.Options{
		URLBase:      urlBase,
		LocalOptions: storage.LocalOptions{Root: t.TempDir(), Secret: testSecret},
	})
	if err != nil {
		t.Fatal(err)
	}

	return mustAdapter(t, s)
}

func newConfigured(t *testing.T, def *storage.Policy, buckets map[storage.BucketName]storage.Policy) *localAdapter {
	t.Helper()

	s, err := New(storage.Options{
		URLBase:      "https://cdn.example.com/base",
		LocalOptions: storage.LocalOptions{Root: t.TempDir(), Secret: testSecret},
		Policy:       &storage.PolicyConfig{Default: def, Buckets: buckets},
	})
	if err != nil {
		t.Fatal(err)
	}

	return mustAdapter(t, s)
}

func ctxWithSubject(t *testing.T, name string) context.Context {
	t.Helper()
	return storage.WithSubject(t.Context(), storage.Subject(name))
}

func writeObject(t *testing.T, a *localAdapter, bucket, key, body string) string {
	t.Helper()

	full, ok := a.resolve(bucket, key)
	if !ok {
		t.Fatalf("resolve failed for %s/%s", bucket, key)
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return full
}

// fileAsBucket replaces root/bucket with a regular file so every Stat under
// it fails with ENOTDIR (a non-NotExist error).
func fileAsBucket(t *testing.T, a *localAdapter, bucket string) {
	t.Helper()

	p := filepath.Join(a.root, bucket)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(p, []byte("not-a-dir"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func needsCheckPolicy() *storage.Policy {
	return &storage.Policy{
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"bob"}},
	}
}

type failReader struct{ err error }

func (r failReader) Read(_ []byte) (int, error) { return 0, r.err }

// ---------------------------------------------------------------------------
// New.
// ---------------------------------------------------------------------------

func TestLocalNewValidateError(t *testing.T) {
	// Bad base URL is rejected by Options.Validate.
	_, err := New(storage.Options{
		URLBase:      "ftp://example.com",
		LocalOptions: storage.LocalOptions{Root: t.TempDir(), Secret: testSecret},
	})
	if err == nil {
		t.Fatal("expected error for non-http base URL")
	}

	// Bad policy is rejected by Options.Validate.
	_, err = New(storage.Options{
		LocalOptions: storage.LocalOptions{Root: t.TempDir(), Secret: testSecret},
		Policy: &storage.PolicyConfig{Default: &storage.Policy{
			Write: storage.Rule{Allow: []storage.Subject{""}},
		}},
	})
	if err == nil {
		t.Fatal("expected error for invalid policy")
	}
}

func TestLocalNewDefaultRoot(t *testing.T) {
	// Root "" falls back to /tmp/storage. Preserve any pre-existing state.
	const defRoot = "/tmp/storage"

	var prevMode os.FileMode

	hadRoot := false

	if fi, statErr := os.Stat(defRoot); statErr == nil {
		hadRoot = true
		prevMode = fi.Mode().Perm()
	}

	s, err := New(storage.Options{
		LocalOptions: storage.LocalOptions{Secret: testSecret},
	})
	if err != nil {
		t.Fatal(err)
	}

	a := mustAdapter(t, s)
	if a.root != defRoot {
		t.Fatalf("root = %q, want %q", a.root, defRoot)
	}

	if hadRoot {
		if err := os.Chmod(defRoot, prevMode); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.RemoveAll(defRoot); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocalNewEphemeralSecret(t *testing.T) {
	s, err := New(storage.Options{
		LocalOptions: storage.LocalOptions{Root: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}

	a := mustAdapter(t, s)
	if len(a.secret) == 0 {
		t.Fatal("expected generated secret")
	}

	// Generated secret must be usable for presigning.
	if _, err := s.PresignDownload(t.Context(), "bkt", "k", time.Hour); err != nil {
		t.Fatal(err)
	}
}

func TestLocalNewEphemeralSecretRandError(t *testing.T) {
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("rand boom") }
	defer func() { randRead = old }()

	_, err := New(storage.Options{
		LocalOptions: storage.LocalOptions{Root: t.TempDir()},
	})
	if err == nil || !strings.Contains(err.Error(), "generate secret") {
		t.Fatalf("expected generate secret error, got %v", err)
	}
}

func TestLocalNewProductionRequiresSecret(t *testing.T) {
	t.Setenv("ZEVER_ENV", "production")

	_, err := New(storage.Options{
		LocalOptions: storage.LocalOptions{Root: t.TempDir()},
	})
	if err == nil || !strings.Contains(err.Error(), "required in production") {
		t.Fatalf("expected production secret error, got %v", err)
	}
}

func TestLocalNewMkdirFails(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := New(storage.Options{
		LocalOptions: storage.LocalOptions{Root: filepath.Join(blocker, "sub"), Secret: testSecret},
	})
	if err == nil || !strings.Contains(err.Error(), "mkdir root") {
		t.Fatalf("expected mkdir root error, got %v", err)
	}
}

func TestLocalNewRootPerms(t *testing.T) {
	pubRead := &storage.Policy{Read: storage.Rule{Public: true}}
	priv := &storage.Policy{Write: storage.Rule{Allow: []storage.Subject{"alice"}}}
	writeOnly := &storage.Policy{
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}

	cases := []struct {
		name   string
		policy *storage.PolicyConfig
		bucket map[storage.BucketName]storage.Policy
		want   os.FileMode
	}{
		{"unconfigured", nil, nil, 0o700},
		{"per-bucket-public", nil, map[storage.BucketName]storage.Policy{"pub": {Read: storage.Rule{Public: true}}}, 0o750},
		{"default-public", &storage.PolicyConfig{Default: pubRead}, nil, 0o750},
		{"configured-private", &storage.PolicyConfig{Default: priv}, nil, 0o700},
		{"public-write-only", &storage.PolicyConfig{Default: writeOnly}, nil, 0o700},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()

			cfg := tc.policy
			if cfg == nil && tc.bucket != nil {
				cfg = &storage.PolicyConfig{Buckets: tc.bucket}
			}

			s, err := New(storage.Options{
				LocalOptions: storage.LocalOptions{Root: root, Secret: testSecret},
				Policy:       cfg,
			})
			if err != nil {
				t.Fatal(err)
			}

			_ = s

			fi, err := os.Stat(root)
			if err != nil {
				t.Fatal(err)
			}

			if got := fi.Mode().Perm(); got != tc.want {
				t.Fatalf("root perm = %o, want %o", got, tc.want)
			}
		})
	}
}

func TestLocalNewExistingRootRechmod(t *testing.T) {
	root := t.TempDir()

	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := New(storage.Options{
		LocalOptions: storage.LocalOptions{Root: root, Secret: testSecret},
		Policy: &storage.PolicyConfig{Default: &storage.Policy{
			Read: storage.Rule{Public: true},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}

	if got := fi.Mode().Perm(); got != 0o750 {
		t.Fatalf("root perm = %o, want 750", got)
	}
}

// ---------------------------------------------------------------------------
// uploadPerm.
// ---------------------------------------------------------------------------

func TestLocalCoreUploadPermNoCheck(t *testing.T) {
	a := newUnconfigured(t, "")

	// Zero policy has identical write/update rules: no existence check.
	perm, err := a.uploadPerm("bkt", "missing", storage.Policy{})
	if err != nil {
		t.Fatal(err)
	}

	if perm != storage.PermWrite {
		t.Fatalf("perm = %q, want write", perm)
	}
}

func TestLocalCoreUploadPermExistence(t *testing.T) {
	a := newUnconfigured(t, "")
	pol := *needsCheckPolicy()

	perm, err := a.uploadPerm("bkt", "missing", pol)
	if err != nil {
		t.Fatal(err)
	}

	if perm != storage.PermWrite {
		t.Fatalf("missing perm = %q, want write", perm)
	}

	writeObject(t, a, "bkt", "present", "data")

	perm, err = a.uploadPerm("bkt", "present", pol)
	if err != nil {
		t.Fatal(err)
	}

	if perm != storage.PermUpdate {
		t.Fatalf("present perm = %q, want update", perm)
	}
}

func TestLocalCoreUploadPermStatError(t *testing.T) {
	a := newUnconfigured(t, "")
	fileAsBucket(t, a, "bkt")

	if _, err := a.uploadPerm("bkt", "k", *needsCheckPolicy()); err == nil {
		t.Fatal("expected stat error")
	}
}

// ---------------------------------------------------------------------------
// PresignUpload.
// ---------------------------------------------------------------------------

func TestLocalCorePresignUploadValidate(t *testing.T) {
	a := newUnconfigured(t, "")

	if _, err := a.PresignUpload(t.Context(), "", "k", "", time.Hour); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Fatalf("expected ErrInvalidBucket, got %v", err)
	}

	if _, err := a.PresignUpload(t.Context(), "bkt", "../x", "", time.Hour); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}

	if _, err := a.PresignUpload(t.Context(), "bkt", "k", "", 0); err == nil {
		t.Fatal("expected expiry error for zero ttl")
	}

	if _, err := a.PresignUpload(t.Context(), "bkt", "k", "", 8*24*time.Hour); err == nil {
		t.Fatal("expected expiry error for oversize ttl")
	}
}

func TestLocalCorePresignUploadUnconfigured(t *testing.T) {
	a := newUnconfigured(t, "https://cdn.example.com/base")

	// Anonymous: no sub param, still signed.
	got, err := a.PresignUpload(t.Context(), "bkt", "k", "text/plain", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if got.Method != http.MethodPut {
		t.Fatalf("method = %q, want PUT", got.Method)
	}

	for _, want := range []string{"cdn.example.com", "/base/bkt/k", "sig=", "expires=", "ct=text%2Fplain"} {
		if !strings.Contains(got.URL, want) {
			t.Fatalf("URL %q missing %q", got.URL, want)
		}
	}

	if strings.Contains(got.URL, "sub=") {
		t.Fatalf("anonymous URL should not carry sub: %q", got.URL)
	}

	// With subject: sub param is bound into the URL.
	got, err = a.PresignUpload(ctxWithSubject(t, "alice"), "bkt", "k", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got.URL, "sub=alice") {
		t.Fatalf("URL %q missing sub=alice", got.URL)
	}
}

func TestLocalCorePresignUploadPublicStatic(t *testing.T) {
	a := newConfigured(t, &storage.Policy{
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}, nil)

	got, err := a.PresignUpload(t.Context(), "bkt", "k", "text/plain", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if got.Method != http.MethodPut {
		t.Fatalf("method = %q, want PUT", got.Method)
	}

	if strings.Contains(got.URL, "sig=") {
		t.Fatalf("public URL should be unsigned: %q", got.URL)
	}

	if want := a.staticURL("bkt", "k"); got.URL != want {
		t.Fatalf("URL = %q, want static %q", got.URL, want)
	}
}

func TestLocalCorePresignUploadStatError(t *testing.T) {
	a := newConfigured(t, needsCheckPolicy(), nil)
	fileAsBucket(t, a, "bkt")

	if _, err := a.PresignUpload(ctxWithSubject(t, "alice"), "bkt", "k", "", time.Hour); err == nil {
		t.Fatal("expected stat error")
	}
}

func TestLocalCorePresignUploadPolicyDecisions(t *testing.T) {
	a := newConfigured(t, needsCheckPolicy(), nil)

	// Anonymous is forbidden on a private bucket.
	if _, err := a.PresignUpload(t.Context(), "bkt", "k", "", time.Hour); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// Subject without allow entry is forbidden.
	if _, err := a.PresignUpload(ctxWithSubject(t, "mallory"), "bkt", "k", "", time.Hour); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// Allowed subject gets a signed URL.
	got, err := a.PresignUpload(ctxWithSubject(t, "alice"), "bkt", "k", "text/plain", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if got.Method != http.MethodPut {
		t.Fatalf("method = %q, want PUT", got.Method)
	}

	for _, want := range []string{"bkt", "sig=", "expires=", "sub=alice"} {
		if !strings.Contains(got.URL, want) {
			t.Fatalf("URL %q missing %q", got.URL, want)
		}
	}
}

func TestLocalCorePresignUploadWriteVsUpdate(t *testing.T) {
	a := newConfigured(t, needsCheckPolicy(), nil)

	// Missing object: write perm, alice allowed.
	if _, err := a.PresignUpload(ctxWithSubject(t, "alice"), "bkt", "new", "", time.Hour); err != nil {
		t.Fatal(err)
	}

	writeObject(t, a, "bkt", "old", "data")

	// Existing object: update perm, alice is not in update allow.
	if _, err := a.PresignUpload(ctxWithSubject(t, "alice"), "bkt", "old", "", time.Hour); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// Bob holds the update perm.
	if _, err := a.PresignUpload(ctxWithSubject(t, "bob"), "bkt", "old", "", time.Hour); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// PresignDownload.
// ---------------------------------------------------------------------------

func TestLocalCorePresignDownloadValidate(t *testing.T) {
	a := newUnconfigured(t, "")

	if _, err := a.PresignDownload(t.Context(), "", "k", time.Hour); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Fatalf("expected ErrInvalidBucket, got %v", err)
	}

	if _, err := a.PresignDownload(t.Context(), "bkt", "../x", time.Hour); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}

	if _, err := a.PresignDownload(t.Context(), "bkt", "k", -time.Second); err == nil {
		t.Fatal("expected expiry error for negative ttl")
	}

	if _, err := a.PresignDownload(t.Context(), "bkt", "k", 30*24*time.Hour); err == nil {
		t.Fatal("expected expiry error for oversize ttl")
	}
}

func TestLocalCorePresignDownloadUnconfigured(t *testing.T) {
	a := newUnconfigured(t, "https://cdn.example.com")

	got, err := a.PresignDownload(ctxWithSubject(t, "alice"), "bkt", "a/b", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if got.Method != storage.HTTPMethodGET {
		t.Fatalf("method = %q, want GET", got.Method)
	}

	for _, want := range []string{"bkt/a/b", "sig=", "expires=", "sub=alice"} {
		if !strings.Contains(got.URL, want) {
			t.Fatalf("URL %q missing %q", got.URL, want)
		}
	}
}

func TestLocalCorePresignDownloadPublicStatic(t *testing.T) {
	a := newConfigured(t, &storage.Policy{Read: storage.Rule{Public: true}}, nil)

	got, err := a.PresignDownload(t.Context(), "bkt", "k", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if got.Method != storage.HTTPMethodGET {
		t.Fatalf("method = %q, want GET", got.Method)
	}

	if strings.Contains(got.URL, "sig=") {
		t.Fatalf("public URL should be unsigned: %q", got.URL)
	}
}

func TestLocalCorePresignDownloadPolicyDecisions(t *testing.T) {
	a := newConfigured(t, &storage.Policy{Read: storage.Rule{Allow: []storage.Subject{"alice"}}}, nil)

	if _, err := a.PresignDownload(t.Context(), "bkt", "k", time.Hour); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	if _, err := a.PresignDownload(ctxWithSubject(t, "mallory"), "bkt", "k", time.Hour); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	got, err := a.PresignDownload(ctxWithSubject(t, "alice"), "bkt", "k", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got.URL, "sub=alice") || !strings.Contains(got.URL, "sig=") {
		t.Fatalf("signed URL missing sig/sub: %q", got.URL)
	}
}

// ---------------------------------------------------------------------------
// Exists / exists.
// ---------------------------------------------------------------------------

func TestLocalCoreExists(t *testing.T) {
	a := newUnconfigured(t, "")

	if _, err := a.Exists(t.Context(), "", "k"); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Fatalf("expected ErrInvalidBucket, got %v", err)
	}

	found, err := a.Exists(t.Context(), "bkt", "missing")
	if err != nil {
		t.Fatal(err)
	}

	if found {
		t.Fatal("missing object reported as existing")
	}

	writeObject(t, a, "bkt", "present", "data")

	found, err = a.Exists(t.Context(), "bkt", "present")
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatal("present object reported as missing")
	}

	// Non-NotExist stat errors propagate.
	fileAsBucket(t, a, "blocked")

	if _, err := a.Exists(t.Context(), "blocked", "k"); err == nil {
		t.Fatal("expected stat error")
	}
}

func TestLocalCoreExistsHelper(t *testing.T) {
	a := newUnconfigured(t, "")

	// Invalid bucket fails resolve.
	if _, err := a.exists("..", "k"); err == nil {
		t.Fatal("expected resolve error")
	}
}

// ---------------------------------------------------------------------------
// Delete.
// ---------------------------------------------------------------------------

func TestLocalCoreDeleteValidate(t *testing.T) {
	a := newUnconfigured(t, "")

	if err := a.Delete(t.Context(), "", "k"); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Fatalf("expected ErrInvalidBucket, got %v", err)
	}

	if err := a.Delete(t.Context(), "bkt", "../x"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestLocalCoreDeleteMissingIsNil(t *testing.T) {
	a := newUnconfigured(t, "")

	// Missing object and missing bucket both succeed (idempotent delete).
	if err := a.Delete(t.Context(), "nobucket", "nokey"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalCoreDeleteSuccessRemovesMeta(t *testing.T) {
	a := newUnconfigured(t, "")

	full := writeObject(t, a, "bkt", "k", "data")

	if err := os.WriteFile(full+metaSuffix, []byte(`{"contentType":"text/plain"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.Delete(t.Context(), "bkt", "k"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(full); !os.IsNotExist(err) {
		t.Fatalf("object still present: %v", err)
	}

	if _, err := os.Stat(full + metaSuffix); !os.IsNotExist(err) {
		t.Fatalf("sidecar still present: %v", err)
	}
}

func TestLocalCoreDeletePolicyDeny(t *testing.T) {
	a := newConfigured(t, &storage.Policy{}, nil)
	writeObject(t, a, "bkt", "k", "data")

	if err := a.Delete(ctxWithSubject(t, "alice"), "bkt", "k"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// Allowed subject succeeds.
	ok := newConfigured(t, &storage.Policy{Delete: storage.Rule{Allow: []storage.Subject{"alice"}}}, nil)
	writeObject(t, ok, "bkt", "k", "data")

	if err := ok.Delete(ctxWithSubject(t, "alice"), "bkt", "k"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalCoreDeleteSymlinkEscape(t *testing.T) {
	a := newUnconfigured(t, "")

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(a.root, "link")); err != nil {
		t.Fatal(err)
	}

	if err := a.Delete(t.Context(), "link", "k"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestLocalCoreDeleteNonEmptyDir(t *testing.T) {
	a := newUnconfigured(t, "")
	writeObject(t, a, "bkt", "dir/file", "data")

	err := a.Delete(t.Context(), "bkt", "dir")
	if err == nil || !strings.Contains(err.Error(), "delete") {
		t.Fatalf("expected delete error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Move.
// ---------------------------------------------------------------------------

func TestLocalCoreMoveValidate(t *testing.T) {
	a := newUnconfigured(t, "")

	if err := a.Move(t.Context(), "", "k", "b", "k"); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Fatalf("expected ErrInvalidBucket, got %v", err)
	}

	if err := a.Move(t.Context(), "b", "k", "b", "../x"); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestLocalCoreMoveSelfNoop(t *testing.T) {
	a := newUnconfigured(t, "")

	// Self-move is a no-op even when the source does not exist.
	if err := a.Move(t.Context(), "bkt", "k", "bkt", "k"); err != nil {
		t.Fatal(err)
	}

	writeObject(t, a, "bkt", "k", "data")

	if err := a.Move(t.Context(), "bkt", "k", "bkt", "k"); err != nil {
		t.Fatal(err)
	}

	if _, err := a.Exists(t.Context(), "bkt", "k"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalCoreMoveMissingSource(t *testing.T) {
	a := newUnconfigured(t, "")

	if err := a.Move(t.Context(), "bkt", "missing", "bkt", "dst"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalCoreMoveSymlinkSource(t *testing.T) {
	a := newUnconfigured(t, "")
	target := writeObject(t, a, "bkt", "real", "data")

	link, ok := a.resolve("bkt", "link")
	if !ok {
		t.Fatal("resolve failed")
	}

	_ = target

	if err := os.Symlink(filepath.Join(a.root, "bkt", "real"), link); err != nil {
		t.Fatal(err)
	}

	if err := a.Move(t.Context(), "bkt", "link", "bkt", "dst"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestLocalCoreMoveLstatError(t *testing.T) {
	a := newUnconfigured(t, "")
	fileAsBucket(t, a, "bkt")

	err := a.Move(t.Context(), "bkt", "k", "bkt", "dst")
	if err == nil || !strings.Contains(err.Error(), "stat") {
		t.Fatalf("expected stat error, got %v", err)
	}
}

func TestLocalCoreMoveLexicalEscape(t *testing.T) {
	a := newUnconfigured(t, "")
	writeObject(t, a, "bkt", "k", "data")

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(a.root, "link")); err != nil {
		t.Fatal(err)
	}

	if err := a.Move(t.Context(), "bkt", "k", "link", "dst"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestLocalCoreMovePolicyDecisions(t *testing.T) {
	mkdeny := func(t *testing.T, def storage.Policy) *localAdapter {
		t.Helper()

		return newConfigured(t, &def, nil)
	}

	// Empty policy denies reads.
	a := mkdeny(t, storage.Policy{})
	writeObject(t, a, "src", "k", "data")

	if err := a.Move(ctxWithSubject(t, "alice"), "src", "k", "dst", "k"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden on read, got %v", err)
	}

	// Read allowed, delete denied.
	a = mkdeny(t, storage.Policy{Read: storage.Rule{Allow: []storage.Subject{"alice"}}})
	writeObject(t, a, "src", "k", "data")

	if err := a.Move(ctxWithSubject(t, "alice"), "src", "k", "dst", "k"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden on delete, got %v", err)
	}

	// Read+delete allowed, dst write denied.
	a = mkdeny(t, storage.Policy{
		Read:   storage.Rule{Allow: []storage.Subject{"alice"}},
		Delete: storage.Rule{Allow: []storage.Subject{"alice"}},
	})
	writeObject(t, a, "src", "k", "data")

	if err := a.Move(ctxWithSubject(t, "alice"), "src", "k", "dst", "k"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden on dst write, got %v", err)
	}

	// All allowed: succeeds.
	a = mkdeny(t, storage.Policy{
		Read:   storage.Rule{Allow: []storage.Subject{"alice"}},
		Delete: storage.Rule{Allow: []storage.Subject{"alice"}},
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"alice"}},
	})
	writeObject(t, a, "src", "k", "data")

	if err := a.Move(ctxWithSubject(t, "alice"), "src", "k", "dst", "k"); err != nil {
		t.Fatal(err)
	}

	found, err := a.Exists(ctxWithSubject(t, "alice"), "dst", "k")
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatal("dst missing after move")
	}
}

func TestLocalCoreMoveDstExistCheck(t *testing.T) {
	a := newConfigured(t, &storage.Policy{
		Read:   storage.Rule{Allow: []storage.Subject{"alice", "bob"}},
		Delete: storage.Rule{Allow: []storage.Subject{"alice", "bob"}},
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"bob"}},
	}, nil)

	writeObject(t, a, "src", "k", "data")

	// Dst missing: write perm, alice allowed.
	if err := a.Move(ctxWithSubject(t, "alice"), "src", "k", "dst", "fresh"); err != nil {
		t.Fatal(err)
	}

	writeObject(t, a, "src", "k2", "data")
	writeObject(t, a, "dst", "taken", "old")

	// Dst present: update perm, alice is not in update allow.
	if err := a.Move(ctxWithSubject(t, "alice"), "src", "k2", "dst", "taken"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// Bob holds the update perm.
	if err := a.Move(ctxWithSubject(t, "bob"), "src", "k2", "dst", "taken"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalCoreMoveDstStatError(t *testing.T) {
	a := newConfigured(t, &storage.Policy{
		Read:   storage.Rule{Allow: []storage.Subject{"alice"}},
		Delete: storage.Rule{Allow: []storage.Subject{"alice"}},
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"nobody"}},
	}, nil)

	writeObject(t, a, "src", "k", "data")
	fileAsBucket(t, a, "dst")

	err := a.Move(ctxWithSubject(t, "alice"), "src", "k", "dst", "k")
	if err == nil || !strings.Contains(err.Error(), "stat") {
		t.Fatalf("expected stat error, got %v", err)
	}
}

func TestLocalCoreMoveMkdirError(t *testing.T) {
	a := newUnconfigured(t, "")
	writeObject(t, a, "src", "k", "data")

	// root/dst exists as a dir but root/dst/sub is a regular file: the
	// lexical pre-checks pass, then MkdirAll fails with ENOTDIR.
	dstDir, ok := a.resolve("dst", "sub")
	if !ok {
		t.Fatal("resolve failed")
	}

	if err := os.MkdirAll(filepath.Dir(dstDir), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(dstDir, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := a.Move(t.Context(), "src", "k", "dst", "sub/k")
	if err == nil || !strings.Contains(err.Error(), "mkdir") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestLocalCoreMoveRenameError(t *testing.T) {
	a := newUnconfigured(t, "")
	writeObject(t, a, "src", "k", "data")

	// Dst path is an existing empty directory: Rename(file -> dir) fails.
	dstFull, ok := a.resolve("dst", "dir")
	if !ok {
		t.Fatal("resolve failed")
	}

	if err := os.MkdirAll(dstFull, 0o700); err != nil {
		t.Fatal(err)
	}

	err := a.Move(t.Context(), "src", "k", "dst", "dir")
	if err == nil || !strings.Contains(err.Error(), "move") {
		t.Fatalf("expected move error, got %v", err)
	}
}

func TestLocalCoreMoveSuccessWithMeta(t *testing.T) {
	a := newUnconfigured(t, "")

	srcFull := writeObject(t, a, "src", "k", "data")

	if err := os.WriteFile(srcFull+metaSuffix, []byte(`{"contentType":"text/plain"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.Move(t.Context(), "src", "k", "dst", "sub/k"); err != nil {
		t.Fatal(err)
	}

	dstFull, ok := a.resolve("dst", "sub/k")
	if !ok {
		t.Fatal("resolve failed")
	}

	body, err := os.ReadFile(dstFull)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "data" {
		t.Fatalf("dst body = %q, want data", body)
	}

	if _, err := os.Stat(dstFull + metaSuffix); err != nil {
		t.Fatalf("dst sidecar missing: %v", err)
	}

	if _, err := os.Stat(srcFull); !os.IsNotExist(err) {
		t.Fatalf("src still present: %v", err)
	}

	if _, err := os.Stat(srcFull + metaSuffix); !os.IsNotExist(err) {
		t.Fatalf("src sidecar still present: %v", err)
	}
}

func TestLocalCoreMoveClearsStaleDstMeta(t *testing.T) {
	a := newUnconfigured(t, "")

	writeObject(t, a, "src", "k", "data")

	// Stale dst object with a sidecar; src has none.
	dstFull := writeObject(t, a, "dst", "k", "old")

	if err := os.WriteFile(dstFull+metaSuffix, []byte(`{"contentType":"text/plain"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := a.Move(t.Context(), "src", "k", "dst", "k"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dstFull + metaSuffix); !os.IsNotExist(err) {
		t.Fatalf("stale dst sidecar still present: %v", err)
	}
}

func TestLocalCoreMoveMetaSidecar(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "sub", "dst")

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}

	// Present: renamed.
	if err := os.WriteFile(src+metaSuffix, []byte("meta"), 0o600); err != nil {
		t.Fatal(err)
	}

	moveMetaSidecar(src, dst)

	body, err := os.ReadFile(dst + metaSuffix)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "meta" {
		t.Fatalf("sidecar = %q, want meta", body)
	}

	// Absent with stale dst: removed.
	if err := os.WriteFile(dst+metaSuffix, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	moveMetaSidecar(src, dst)

	if _, err := os.Stat(dst + metaSuffix); !os.IsNotExist(err) {
		t.Fatalf("stale sidecar still present: %v", err)
	}

	// Absent with no dst: no-op, no error possible (void return).
	moveMetaSidecar(src, dst)
}

// ---------------------------------------------------------------------------
// resolve / containment.
// ---------------------------------------------------------------------------

func TestLocalCoreResolve(t *testing.T) {
	a := newUnconfigured(t, "")

	full, ok := a.resolve("bkt", "a/b")
	if !ok {
		t.Fatal("resolve failed")
	}

	if want := filepath.Join(a.root, "bkt", "a", "b"); full != want {
		t.Fatalf("full = %q, want %q", full, want)
	}

	if _, ok2 := a.resolve("..", "k"); ok2 {
		t.Fatal("expected resolve to reject outside-root bucket")
	}

	// Key ".." cleans to the bucket dir itself (Join lexically); callers
	// rely on ValidKey to reject it before resolve, and on
	// lexicallyContained afterwards.
	full, ok = a.resolve("bkt", "..")
	if !ok {
		t.Fatal("expected resolve to clean parent key to bucket dir")
	}

	if want := a.root; full != want {
		t.Fatalf("full = %q, want %q", full, want)
	}
}

func TestLocalCoreContainedRel(t *testing.T) {
	root := string(filepath.Separator) + "root"

	if !containedRel(root, filepath.Join(root, "b", "k")) {
		t.Fatal("expected inside path to be contained")
	}

	if containedRel(root, filepath.Join(root, "..", "other")) {
		t.Fatal("expected sibling path to escape")
	}

	// Rel(root, root) is "." which passes every containedRel check.
	if !containedRel(root, root) {
		t.Fatal("expected root itself to be contained (rel is .)")
	}

	// Rel errors on mixed relative/absolute inputs.
	if containedRel("rel", string(filepath.Separator)+"abs") {
		t.Fatal("expected Rel error to report uncontained")
	}

	// Absolute rel result is uncontained.
	if containedRel(root, string(filepath.Separator)+"elsewhere") {
		t.Fatal("expected absolute escape to be uncontained")
	}
}

func TestLocalCoreLexicallyContained(t *testing.T) {
	a := newUnconfigured(t, "")

	// Existing directory inside root.
	full := writeObject(t, a, "bkt", "k", "data")
	if !a.lexicallyContained(full) {
		t.Fatal("expected contained for inside path")
	}

	// Missing directory falls back to the lexical check (not an escape).
	missing := filepath.Join(a.root, "nobucket", "k")
	if !a.lexicallyContained(missing) {
		t.Fatal("expected lexical fallback for missing dir")
	}

	// Symlink escaping root.
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(a.root, "link")); err != nil {
		t.Fatal(err)
	}

	if a.lexicallyContained(filepath.Join(a.root, "link", "k")) {
		t.Fatal("expected symlink escape to be uncontained")
	}

	// Non-NotExist EvalSymlinks error (path through a regular file).
	blocker := filepath.Join(a.root, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if a.lexicallyContained(filepath.Join(a.root, "file", "deep", "k")) {
		t.Fatal("expected ENOTDIR traversal to be uncontained")
	}
}

func TestLocalCoreLexicallyContainedRootGone(t *testing.T) {
	a := newUnconfigured(t, "")

	outsideDir := t.TempDir()

	if err := os.RemoveAll(a.root); err != nil {
		t.Fatal(err)
	}

	// Dir exists, but the root no longer resolves: realContained fails.
	if a.lexicallyContained(filepath.Join(outsideDir, "k")) {
		t.Fatal("expected uncontained when root is gone")
	}
}

func TestLocalCoreRealContained(t *testing.T) {
	a := newUnconfigured(t, "")

	rootReal, err := a.getRootReal()
	if err != nil {
		t.Fatal(err)
	}

	if !a.realContained(filepath.Join(rootReal, "bkt")) {
		t.Fatal("expected inside path to be contained")
	}

	if a.realContained(filepath.Join(rootReal, "..", "other")) {
		t.Fatal("expected outside path to be uncontained")
	}

	// Root removed on a fresh adapter: getRootReal errors.
	b := newUnconfigured(t, "")

	if err := os.RemoveAll(b.root); err != nil {
		t.Fatal(err)
	}

	if b.realContained(filepath.Join(t.TempDir(), "k")) {
		t.Fatal("expected uncontained when root is gone")
	}
}

func TestLocalCoreGetRootReal(t *testing.T) {
	a := newUnconfigured(t, "")

	first, err := a.getRootReal()
	if err != nil {
		t.Fatal(err)
	}

	second, err := a.getRootReal()
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatalf("getRootReal not cached: %q vs %q", first, second)
	}

	b := newUnconfigured(t, "")

	if err := os.RemoveAll(b.root); err != nil {
		t.Fatal(err)
	}

	if _, err := b.getRootReal(); err == nil {
		t.Fatal("expected error for removed root")
	}
}

func TestLocalCoreLexicalContainedNoEval(t *testing.T) {
	a := newUnconfigured(t, "")

	full, ok := a.resolve("bkt", "k")
	if !ok {
		t.Fatal("resolve failed")
	}

	if !a.lexicalContainedNoEval(full) {
		t.Fatal("expected inside path to be contained")
	}

	if a.lexicalContainedNoEval(filepath.Join(a.root, "..", "escape")) {
		t.Fatal("expected escape to be uncontained")
	}

	// Root removed: falls back to the lexical root.
	b := newUnconfigured(t, "")

	if err := os.RemoveAll(b.root); err != nil {
		t.Fatal(err)
	}

	if !b.lexicalContainedNoEval(filepath.Join(b.root, "bkt", "k")) {
		t.Fatal("expected lexical-root fallback to contain inside path")
	}
}

// ---------------------------------------------------------------------------
// URL helpers.
// ---------------------------------------------------------------------------

func TestLocalCoreObjectURL(t *testing.T) {
	a := newUnconfigured(t, "https://cdn.example.com/pfx/")

	u := a.objectURL("bkt", "a/b c")
	if u.Path != "/pfx/bkt/a/b c" {
		t.Fatalf("path = %q", u.Path)
	}

	if !strings.Contains(u.RawPath, "b%20c") {
		t.Fatalf("RawPath = %q, want escaped space", u.RawPath)
	}

	plain := newUnconfigured(t, "")
	if got := plain.objectURL("bkt", "k").Path; got != "/bkt/k" {
		t.Fatalf("path = %q, want /bkt/k", got)
	}

	if got := plain.staticURL("bkt", "k"); got != "/bkt/k" {
		t.Fatalf("staticURL = %q, want /bkt/k", got)
	}
}

func TestLocalCoreEscapePath(t *testing.T) {
	if got := escapePath("bkt", "a/b c"); got != "bkt/a/b%20c" {
		t.Fatalf("escapePath = %q", got)
	}
}

func TestLocalCoreSignAndURLFor(t *testing.T) {
	a := newUnconfigured(t, "https://cdn.example.com")

	sig := a.sign("PUT", "bkt", "k", "text/plain", 123, "alice")
	if len(sig) != 64 {
		t.Fatalf("sig len = %d, want 64 hex chars", len(sig))
	}

	// Deterministic for identical inputs.
	if again := a.sign("PUT", "bkt", "k", "text/plain", 123, "alice"); again != sig {
		t.Fatal("sign not deterministic")
	}

	// Full query: ct + sub.
	u, err := url.Parse(a.urlFor("bkt", "k", "text/plain", 123, sig, "alice"))
	if err != nil {
		t.Fatal(err)
	}

	q := u.Query()
	if q.Get("sig") != sig || q.Get("sub") != "alice" || q.Get("ct") != "text/plain" || q.Get("expires") != "123" {
		t.Fatalf("query = %q", u.RawQuery)
	}

	// Minimal query: no ct, no sub.
	u, err = url.Parse(a.urlFor("bkt", "k", "", 123, sig, ""))
	if err != nil {
		t.Fatal(err)
	}

	q = u.Query()
	if q.Get("sig") != sig || q.Get("expires") != "123" {
		t.Fatalf("query = %q", u.RawQuery)
	}

	if q.Has("ct") || q.Has("sub") {
		t.Fatalf("minimal query should omit ct/sub: %q", u.RawQuery)
	}
}

// ---------------------------------------------------------------------------
// Modes.
// ---------------------------------------------------------------------------

func TestLocalCoreDirMode(t *testing.T) {
	plain := newUnconfigured(t, "")
	if got := plain.dirMode("any"); got != 0o700 {
		t.Fatalf("unconfigured dirMode = %o, want 700", got)
	}

	a := newConfigured(t, &storage.Policy{Read: storage.Rule{Public: true}}, nil)
	if got := a.dirMode("bkt"); got != 0o750 {
		t.Fatalf("public dirMode = %o, want 750", got)
	}

	priv := newConfigured(t, &storage.Policy{}, nil)
	if got := priv.dirMode("bkt"); got != 0o700 {
		t.Fatalf("private dirMode = %o, want 700", got)
	}

	// Per-bucket policy wins over a private default.
	mixed := newConfigured(t, &storage.Policy{}, map[storage.BucketName]storage.Policy{
		"pub": {Read: storage.Rule{Public: true}},
	})
	if got := mixed.dirMode("pub"); got != 0o750 {
		t.Fatalf("per-bucket public dirMode = %o, want 750", got)
	}

	if got := mixed.dirMode("other"); got != 0o700 {
		t.Fatalf("default private dirMode = %o, want 700", got)
	}
}

func TestLocalCoreDirModeForPut(t *testing.T) {
	plain := newUnconfigured(t, "")
	full, ok := plain.resolve("bkt", "sub/k")
	if !ok {
		t.Fatal("resolve failed")
	}

	if got := plain.dirModeForPut(full); got != 0o700 {
		t.Fatalf("unconfigured = %o, want 700", got)
	}

	a := newConfigured(t, &storage.Policy{Read: storage.Rule{Public: true}}, nil)
	pfull, ok := a.resolve("bkt", "sub/k")
	if !ok {
		t.Fatal("resolve failed")
	}

	if got := a.dirModeForPut(pfull); got != 0o750 {
		t.Fatalf("public = %o, want 750", got)
	}
}

func TestLocalCoreRootMode(t *testing.T) {
	plain := newUnconfigured(t, "")
	if got := plain.rootMode(); got != 0o700 {
		t.Fatalf("unconfigured rootMode = %o, want 700", got)
	}

	pub := newConfigured(t, &storage.Policy{Read: storage.Rule{Public: true}}, nil)
	if got := pub.rootMode(); got != 0o750 {
		t.Fatalf("public rootMode = %o, want 750", got)
	}

	priv := newConfigured(t, &storage.Policy{}, nil)
	if got := priv.rootMode(); got != 0o700 {
		t.Fatalf("private rootMode = %o, want 700", got)
	}

	// Per-bucket public read widens the root even with a private default.
	mixed := newConfigured(t, &storage.Policy{}, map[storage.BucketName]storage.Policy{
		"pub": {Read: storage.Rule{Public: true}},
	})
	if got := mixed.rootMode(); got != 0o750 {
		t.Fatalf("mixed rootMode = %o, want 750", got)
	}
}

func TestLocalCoreAnyPublicRead(t *testing.T) {
	plain := newUnconfigured(t, "")
	if plain.anyPublicRead() {
		t.Fatal("unconfigured should not report public read")
	}

	pub := newConfigured(t, &storage.Policy{Read: storage.Rule{Public: true}}, nil)
	if !pub.anyPublicRead() {
		t.Fatal("default public read not detected")
	}

	perBucket := newConfigured(t, &storage.Policy{}, map[storage.BucketName]storage.Policy{
		"pub": {Read: storage.Rule{Public: true}},
	})
	if !perBucket.anyPublicRead() {
		t.Fatal("per-bucket public read not detected")
	}

	priv := newConfigured(t, &storage.Policy{Write: storage.Rule{Public: true}}, nil)
	if priv.anyPublicRead() {
		t.Fatal("write-only public misreported as public read")
	}
}

func TestLocalCoreUploadMode(t *testing.T) {
	plain := newUnconfigured(t, "")
	if got := plain.uploadMode("bkt"); got != 0o600 {
		t.Fatalf("unconfigured uploadMode = %o, want 600", got)
	}

	pub := newConfigured(t, &storage.Policy{
		Read:   storage.Rule{Public: true},
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}, nil)
	if got := pub.uploadMode("bkt"); got != 0o644 {
		t.Fatalf("public uploadMode = %o, want 644", got)
	}

	// Public write without public read stays private on disk.
	writeOnly := newConfigured(t, &storage.Policy{
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}, nil)
	if got := writeOnly.uploadMode("bkt"); got != 0o600 {
		t.Fatalf("write-only uploadMode = %o, want 600", got)
	}

	// Public read with private write stays private on disk.
	readOnly := newConfigured(t, &storage.Policy{Read: storage.Rule{Public: true}}, nil)
	if got := readOnly.uploadMode("bkt"); got != 0o600 {
		t.Fatalf("read-only uploadMode = %o, want 600", got)
	}

	// Read + update public (write private) still widens: update covers overwrites.
	readUpdate := newConfigured(t, &storage.Policy{
		Read:   storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}, nil)
	if got := readUpdate.uploadMode("bkt"); got != 0o644 {
		t.Fatalf("read+update uploadMode = %o, want 644", got)
	}
}

// ---------------------------------------------------------------------------
// ensurePutDir.
// ---------------------------------------------------------------------------

func TestLocalCoreEnsurePutDir(t *testing.T) {
	a := newUnconfigured(t, "")

	full, ok := a.resolve("bkt", "sub/dir/k")
	if !ok {
		t.Fatal("resolve failed")
	}

	dir, err := a.ensurePutDir(full)
	if err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !fi.IsDir() {
		t.Fatal("ensurePutDir did not create a directory")
	}

	if got := fi.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir perm = %o, want 700", got)
	}

	// Outside root is forbidden.
	if _, err := a.ensurePutDir(filepath.Join(a.root, "..", "escape", "k")); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestLocalCoreEnsurePutDirPublicMode(t *testing.T) {
	a := newConfigured(t, &storage.Policy{Read: storage.Rule{Public: true}}, nil)

	full, ok := a.resolve("bkt", "sub/k")
	if !ok {
		t.Fatal("resolve failed")
	}

	dir, err := a.ensurePutDir(full)
	if err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got := fi.Mode().Perm(); got != 0o750 {
		t.Fatalf("dir perm = %o, want 750", got)
	}
}

func TestLocalCoreEnsurePutDirSymlinkEscape(t *testing.T) {
	a := newUnconfigured(t, "")

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(a.root, "link")); err != nil {
		t.Fatal(err)
	}

	full, ok := a.resolve("link", "sub/k")
	if !ok {
		t.Fatal("resolve failed")
	}

	// Lexical pre-check passes, but post-mkdir symlink resolution fails.
	if _, err := a.ensurePutDir(full); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeBodyTemp / writeMetaTemp.
// ---------------------------------------------------------------------------

func TestLocalCoreWriteBodyTemp(t *testing.T) {
	dir := t.TempDir()

	// Missing directory.
	if _, err := writeBodyTemp(filepath.Join(dir, "nope"), strings.NewReader("x"), 0o600); err == nil {
		t.Fatal("expected CreateTemp error")
	}

	// Failing body.
	if _, err := writeBodyTemp(dir, failReader{err: io.ErrUnexpectedEOF}, 0o600); err == nil {
		t.Fatal("expected copy error")
	}

	// Success with mode.
	name, err := writeBodyTemp(dir, strings.NewReader("hello"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}

	fi, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}

	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("tmp perm = %o, want 600", got)
	}
}

func TestLocalCoreWriteBodyTempTooLarge(t *testing.T) {
	old := maxBody
	maxBody = 16
	t.Cleanup(func() { maxBody = old })

	dir := t.TempDir()

	_, err := writeBodyTemp(dir, strings.NewReader(strings.Repeat("x", 32)), 0o600)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Oversize body must not leave a temp file behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected no leftover temp files, found %d", len(entries))
	}
}

func TestLocalCoreWriteMetaTemp(t *testing.T) {
	dir := t.TempDir()

	if got := writeMetaTemp(filepath.Join(dir, "nope"), "text/plain", 0o600); got != "" {
		t.Fatalf("expected empty name on CreateTemp error, got %q", got)
	}

	name := writeMetaTemp(dir, "text/plain", 0o600)
	if name == "" {
		t.Fatal("writeMetaTemp returned empty name")
	}

	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), "text/plain") {
		t.Fatalf("meta = %q, want content type", body)
	}

	fi, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}

	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("tmp perm = %o, want 600", got)
	}
}

// ---------------------------------------------------------------------------
// Close / Name.
// ---------------------------------------------------------------------------

func TestLocalCoreCloseAndName(t *testing.T) {
	a := newUnconfigured(t, "")

	if err := a.Close(t.Context()); err != nil {
		t.Fatal(err)
	}

	if a.Name() != "local" {
		t.Fatalf("Name = %q, want local", a.Name())
	}
}
