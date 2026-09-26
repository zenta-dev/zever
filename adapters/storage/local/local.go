package local

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/storage"
)

type localAdapter struct {
	root    string
	urlBase string
	base    url.URL
	secret  []byte
	storage.PolicyStore
	logger log.Logger

	rootReal     string
	rootRealOnce sync.Once
	rootRealErr  error
}

func (a *localAdapter) log() log.Logger {
	if a != nil && a.logger != nil {
		return a.logger
	}

	return noop.New()
}

// randRead is crypto/rand.Read as a var so tests can inject failures.
var randRead = rand.Read

func New(opts storage.Options) (storage.Storage, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	root := opts.Root
	if root == "" {
		root = "/tmp/storage"
	}

	urlBase := opts.URLBase
	secret := opts.Secret

	var base url.URL
	if u, err := url.Parse(urlBase); err == nil {
		base = *u
	}

	var store storage.PolicyStore
	// opts.Validate above already resolved the same config; this cannot fail.
	_ = store.ResolveFromConfig(opts.Policy)

	// FS perms: private roots 0700, shared (any public read) 0750.
	rootPerm := os.FileMode(0o700)

	if store.Configured() {
		for _, p := range store.Policies() {
			if p.Public(storage.PermRead) {
				rootPerm = 0o750
				break
			}
		}

		if rootPerm == 0o700 && store.Default().Public(storage.PermRead) {
			rootPerm = 0o750
		}
	}

	if err := os.MkdirAll(root, rootPerm); err != nil {
		return nil, fmt.Errorf("local: mkdir root: %w", err)
	}
	// Ensure existing root has correct perms (MkdirAll doesn't chmod existing).
	_ = os.Chmod(root, rootPerm)

	if secret == "" {
		if env := os.Getenv("ZEVER_ENV"); env == "production" || env == "prod" {
			return nil, fmt.Errorf("local: option %q is required in production (secret must be explicit)", "secret")
		}

		logger := opts.Logger
		if logger == nil {
			logger = noop.New()
		}
		logger.Warn().Msg("[storage/local] WARNING: no secret provided; using ephemeral key. Presigned URLs will be invalidated on restart.")

		key := make([]byte, 32)
		if _, err := randRead(key); err != nil {
			return nil, fmt.Errorf("local: generate secret: %w", err)
		}

		secret = hex.EncodeToString(key)
	}

	return &localAdapter{
		root:        root,
		urlBase:     urlBase,
		base:        base,
		secret:      []byte(secret),
		PolicyStore: store,
		logger:      opts.Logger,
	}, nil
}

func (a *localAdapter) uploadPerm(bucket, key string, pol storage.Policy) (storage.Perm, error) {
	perm := storage.PermWrite

	if !pol.NeedsExistCheck() {
		return storage.PermWrite, nil
	}

	exists, err := a.exists(bucket, key)
	if err != nil {
		return perm, err
	}

	return storage.SelectUploadPerm(pol, exists), nil
}

func (a *localAdapter) PresignUpload(
	ctx context.Context,
	bucket string,
	key string,
	contentType string,
	ttl time.Duration,
) (storage.PresignedURL, error) {
	if err := storage.ValidateBucketKey(bucket, key); err != nil {
		return storage.PresignedURL{}, fmt.Errorf("local: %w", err)
	}

	expires, err := storage.PresignExpiry(ttl)
	if err != nil {
		return storage.PresignedURL{}, fmt.Errorf("local: %w", err)
	}

	if !a.Configured() {
		sub, _ := storage.SubjectFrom(ctx)
		sig := a.sign("PUT", bucket, key, contentType, expires, sub)

		return storage.PresignedURL{
			Method: "PUT",
			URL:    a.urlFor(bucket, key, contentType, expires, sig, sub),
		}, nil
	}

	pol := a.PolicyFor(bucket)

	if pol.Public(storage.PermWrite) && pol.Public(storage.PermUpdate) {
		sub, _ := storage.SubjectFrom(ctx)
		storage.FireAllow(ctx, storage.PermWrite, bucket, sub, storage.ReasonOkPublic, pol.Version)

		return storage.PresignedURL{Method: "PUT", URL: a.staticURL(bucket, key)}, nil
	}

	perm, err := a.uploadPerm(bucket, key, pol)
	if err != nil {
		return storage.PresignedURL{}, fmt.Errorf("local: stat %q: %w", key, err)
	}

	subject, sok := storage.SubjectFrom(ctx)
	if !sok {
		storage.FireDeny(ctx, perm, bucket, "", storage.ReasonAnonymousForbidden, pol.Version)

		return storage.PresignedURL{}, fmt.Errorf("local: %w", storage.ErrForbidden)
	}

	if !pol.Allow(perm, subject, sok) {
		storage.FireDeny(ctx, perm, bucket, subject, storage.ReasonNotInAllow, pol.Version)

		return storage.PresignedURL{}, fmt.Errorf("local: %w", storage.ErrForbidden)
	}

	storage.FireAllow(ctx, perm, bucket, subject, storage.ReasonOk, pol.Version)

	sig := a.sign("PUT", bucket, key, contentType, expires, subject)

	return storage.PresignedURL{
		Method: "PUT",
		URL:    a.urlFor(bucket, key, contentType, expires, sig, subject),
	}, nil
}

func (a *localAdapter) PresignDownload(
	ctx context.Context,
	bucket string,
	key string,
	ttl time.Duration,
) (storage.PresignedURL, error) {
	if err := storage.ValidateBucketKey(bucket, key); err != nil {
		return storage.PresignedURL{}, fmt.Errorf("local: %w", err)
	}

	expires, err := storage.PresignExpiry(ttl)
	if err != nil {
		return storage.PresignedURL{}, fmt.Errorf("local: %w", err)
	}

	if a.Configured() {
		pol := a.PolicyFor(bucket)
		perm := storage.PermRead
		subject, sok := storage.SubjectFrom(ctx)

		if pol.Public(perm) {
			storage.FireAllow(ctx, perm, bucket, subject, storage.ReasonOkPublic, pol.Version)

			return storage.PresignedURL{Method: storage.HTTPMethodGET, URL: a.staticURL(bucket, key)}, nil
		}

		if !sok {
			storage.FireDeny(ctx, perm, bucket, "", storage.ReasonAnonymousForbidden, pol.Version)

			return storage.PresignedURL{}, fmt.Errorf("local: %w", storage.ErrForbidden)
		}

		if !pol.Allow(perm, subject, sok) {
			storage.FireDeny(ctx, perm, bucket, subject, storage.ReasonNotInAllow, pol.Version)

			return storage.PresignedURL{}, fmt.Errorf("local: %w", storage.ErrForbidden)
		}

		storage.FireAllow(ctx, perm, bucket, subject, storage.ReasonOk, pol.Version)

		sig := a.sign(storage.HTTPMethodGET, bucket, key, "", expires, subject)

		return storage.PresignedURL{
			Method: storage.HTTPMethodGET,
			URL:    a.urlFor(bucket, key, "", expires, sig, subject),
		}, nil
	}

	sub, _ := storage.SubjectFrom(ctx)
	sig := a.sign(storage.HTTPMethodGET, bucket, key, "", expires, sub)

	return storage.PresignedURL{
		Method: storage.HTTPMethodGET,
		URL:    a.urlFor(bucket, key, "", expires, sig, sub),
	}, nil
}

func (a *localAdapter) Exists(_ context.Context, bucket string, key string) (bool, error) {
	if err := storage.ValidateBucketKey(bucket, key); err != nil {
		return false, fmt.Errorf("local: %w", err)
	}

	return a.exists(bucket, key)
}

func (a *localAdapter) Delete(ctx context.Context, bucket string, key string) error {
	if err := storage.ValidateBucketKey(bucket, key); err != nil {
		return fmt.Errorf("local: %w", err)
	}

	// Validated above, so resolve cannot fail; containment is rechecked below.
	full, _ := a.resolve(bucket, key)

	if !a.lexicallyContained(full) {
		return fmt.Errorf("local: %w", storage.ErrForbidden)
	}

	if a.Configured() {
		pol := a.PolicyFor(bucket)
		subject, sok := storage.SubjectFrom(ctx)

		if !pol.Allow(storage.PermDelete, subject, sok) {
			storage.FireDeny(ctx, storage.PermDelete, bucket, subject, storage.ReasonNotInAllow, pol.Version)

			return fmt.Errorf("local: %w", storage.ErrForbidden)
		}
	}

	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("local: delete %q: %w", key, err)
	}

	_ = os.Remove(full + metaSuffix)

	return nil
}

func (a *localAdapter) Move(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	if err := storage.ValidateBucketKey(srcBucket, srcKey); err != nil {
		return fmt.Errorf("local: %w", err)
	}

	if err := storage.ValidateBucketKey(dstBucket, dstKey); err != nil {
		return fmt.Errorf("local: %w", err)
	}

	if srcBucket == dstBucket && srcKey == dstKey {
		return nil
	}

	// Validated above, so resolve cannot fail; containment is rechecked below.
	srcFull, _ := a.resolve(srcBucket, srcKey)
	dstFull, _ := a.resolve(dstBucket, dstKey)

	if !a.lexicallyContained(srcFull) || !a.lexicallyContained(dstFull) {
		return fmt.Errorf("local: %w", storage.ErrForbidden)
	}

	if a.Configured() {
		polSrc := a.PolicyFor(srcBucket)
		subject, sok := storage.SubjectFrom(ctx)

		if !polSrc.Allow(storage.PermRead, subject, sok) {
			storage.FireDeny(ctx, storage.PermRead, srcBucket, subject, storage.DenyReason(polSrc, storage.PermRead, subject, sok), polSrc.Version)

			return fmt.Errorf("local: %w", storage.ErrForbidden)
		}

		if !polSrc.Allow(storage.PermDelete, subject, sok) {
			storage.FireDeny(ctx, storage.PermDelete, srcBucket, subject, storage.DenyReason(polSrc, storage.PermDelete, subject, sok), polSrc.Version)

			return fmt.Errorf("local: %w", storage.ErrForbidden)
		}

		polDst := a.PolicyFor(dstBucket)

		var exists bool

		if polDst.NeedsExistCheck() {
			var err error

			exists, err = a.exists(dstBucket, dstKey)
			if err != nil {
				return fmt.Errorf("local: stat %q: %w", dstKey, err)
			}
		}

		dstPerm := storage.SelectUploadPerm(polDst, exists)

		if !polDst.Allow(dstPerm, subject, sok) {
			storage.FireDeny(ctx, dstPerm, dstBucket, subject, storage.DenyReason(polDst, dstPerm, subject, sok), polDst.Version)

			return fmt.Errorf("local: %w", storage.ErrForbidden)
		}

		storage.FireAllow(ctx, storage.PermRead, srcBucket, subject, storage.ReasonOk, polSrc.Version)
		storage.FireAllow(ctx, storage.PermDelete, srcBucket, subject, storage.ReasonOk, polSrc.Version)
		storage.FireAllow(ctx, dstPerm, dstBucket, subject, storage.ReasonOk, polDst.Version)
	}

	if fi, err := os.Lstat(srcFull); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("local: %w", storage.ErrNotFound)
		}

		return fmt.Errorf("local: stat %q: %w", srcKey, err)
	} else if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local: %w", storage.ErrForbidden)
	}

	// ensurePutDir's forbidden case is unreachable here: lexicallyContained
	// above already rejected symlink escapes, and MkdirAll failures keep
	// their wrapped form below (%w preserves errors.Is).
	if _, err := a.ensurePutDir(dstFull); err != nil {
		return fmt.Errorf("local: mkdir %q: %w", dstKey, err)
	}

	// src and dst live under the same root, so a cross-device rename cannot
	// happen (a symlinked dst dir is rejected by ensurePutDir above).
	if err := os.Rename(srcFull, dstFull); err != nil {
		return fmt.Errorf("local: move %q: %w", srcKey, err)
	}

	moveMetaSidecar(srcFull, dstFull)

	return nil
}

func (a *localAdapter) Name() string { return "local" }

func moveMetaSidecar(srcFull, dstFull string) {
	if err := os.Rename(srcFull+metaSuffix, dstFull+metaSuffix); err != nil {
		if os.IsNotExist(err) {
			_ = os.Remove(dstFull + metaSuffix)
		}
	}
}

// exists reports whether an object is present under bucket/key, resolving
// traversal-safe.
func (a *localAdapter) exists(bucket, key string) (bool, error) {
	full, ok := a.resolve(bucket, key)
	if !ok {
		return false, fmt.Errorf("local: invalid key %q", key)
	}

	if _, err := os.Stat(full); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("local: stat %q: %w", key, err)
	}

	return true, nil
}

// objectURL builds the path for bucket/key under the precomputed base URL.
func (a *localAdapter) objectURL(bucket, key string) *url.URL {
	u := a.base

	prefix := strings.TrimSuffix(u.Path, "/")
	u.Path = prefix + "/" + bucket + "/" + key
	u.RawPath = prefix + "/" + escapePath(bucket, key)

	return &u
}

// staticURL renders a path-only (unsigned) URL for a public permission.
func (a *localAdapter) staticURL(bucket, key string) string {
	return a.objectURL(bucket, key).String()
}

func (a *localAdapter) Close(_ context.Context) error {
	return nil
}

// sign computes the HMAC-SHA256 signature over the canonical request string:
// method, bucket, key, content type (uploads only; "" for downloads), expiry
// and subject. The host is deliberately absent so a URL remains valid at any base.
// Subject is included only for non-public URLs to bind the capability to its
// intended bearer and prevent IDOR replay by another subject.
func (a *localAdapter) sign(method storage.HTTPMethod, bucket, key, contentType string, expires int64, subject storage.Subject) string {
	h := hmac.New(sha256.New, a.secret)
	_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%d\n%s", method, bucket, key, contentType, expires, string(subject))

	return hex.EncodeToString(h.Sum(nil))
}

func (a *localAdapter) urlFor(bucket, key, contentType string, expires int64, sig string, subject storage.Subject) string {
	u := a.objectURL(bucket, key)

	q := u.Query()
	if contentType != "" {
		q.Set("ct", contentType)
	}

	q.Set("expires", strconv.FormatInt(expires, 10))
	q.Set("sig", sig)

	if subject != storage.Subject("") {
		q.Set("sub", string(subject))
	}

	u.RawQuery = q.Encode()

	return u.String()
}

func escapePath(bucket, key string) string {
	segs := make([]string, 0, 3)
	segs = append(segs, url.PathEscape(bucket))

	for _, s := range strings.Split(key, "/") {
		segs = append(segs, url.PathEscape(s))
	}

	return strings.Join(segs, "/")
}

// containedRel reports whether full stays within root lexically.
func containedRel(root, full string) bool {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// resolve maps a bucket/key pair to an absolute path inside root. It is
// defense in depth: even though bucket and key are validated at mint time,
// the handler re-validates them and this containment check guards against any
// traversal slip.
func (a *localAdapter) resolve(bucket, key string) (string, bool) {
	full := filepath.Join(a.root, bucket, filepath.FromSlash(key))

	if !containedRel(a.root, full) {
		return "", false
	}

	return full, true
}

// lexicallyContained reports whether full stays within root after symlink
// resolution of its deepest existing ancestor directory. A symlink planted
// under root that points outside must not allow reads or writes outside root.
// If the directory does not yet exist (e.g. Delete on a missing bucket) it
// falls back to a lexical check so a non-existent path is not mis-classified
// as an escape.
func (a *localAdapter) lexicallyContained(full string) bool {
	dir := filepath.Dir(full)

	realPath, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return a.lexicalContainedNoEval(full)
		}

		return false
	}

	return a.realContained(realPath)
}

// realContained reports whether the resolved absolute path real remains within
// the resolved storage root.
func (a *localAdapter) realContained(realPath string) bool {
	rootReal, err := a.getRootReal()
	if err != nil {
		return false
	}

	return containedRel(rootReal, realPath)
}

func (a *localAdapter) getRootReal() (string, error) {
	a.rootRealOnce.Do(func() {
		a.rootReal, a.rootRealErr = filepath.EvalSymlinks(a.root)
	})

	return a.rootReal, a.rootRealErr
}

// lexicalContainedNoEval reports whether full is lexically inside root without
// resolving symlinks. Used as pre-mkdir check to avoid MkdirAll following a
// symlinked bucket directory outside root (TOCTOU).
func (a *localAdapter) lexicalContainedNoEval(full string) bool {
	// Fast path: cached rootReal if available, otherwise use lexical root.
	rootReal, err := a.getRootReal()

	root := a.root
	if err == nil {
		root = rootReal
	}

	// Resolve full lexically against root: Rel handles .. segments.
	return containedRel(root, full)
}

func (a *localAdapter) dirMode(bucket string) os.FileMode {
	if !a.Configured() {
		return 0o700
	}

	pol := a.PolicyFor(bucket)
	if pol.Public(storage.PermRead) {
		return 0o750
	}

	return 0o700
}

func (a *localAdapter) anyPublicRead() bool {
	// If any policy is public for read, root needs group read/exec for sharing;
	// otherwise private 0700.
	for _, p := range a.Policies() {
		if p.Public(storage.PermRead) {
			return true
		}
	}

	return a.Default().Public(storage.PermRead)
}

func (a *localAdapter) rootMode() os.FileMode {
	if !a.Configured() {
		return 0o700
	}

	if a.anyPublicRead() {
		return 0o750
	}

	return 0o700
}

func (a *localAdapter) dirModeForPut(full string) os.FileMode {
	// full derives from root via resolve, so Rel shares derivation with root
	// (no abs/rel mismatch possible) and Split never yields zero parts.
	rel, _ := filepath.Rel(a.root, full)
	parts := strings.Split(rel, string(filepath.Separator))
	bucket := parts[0]

	return a.dirMode(bucket)
}

// Handler returns an http.Handler that serves URLs minted by PresignUpload
// and PresignDownload. PUT stores the body under the key, GET streams it back.
func (a *localAdapter) Handler() http.Handler {
	return http.HandlerFunc(a.serve)
}

func (a *localAdapter) stripBase(path string) string {
	if a.urlBase == "" {
		return path
	}

	basePath := strings.TrimSuffix(a.base.Path, "/")
	if basePath != "" && strings.HasPrefix(path, basePath+"/") {
		return strings.TrimPrefix(path, basePath)
	}

	return path
}

func parseBucketKey(path string) (string, string, bool) {
	bucket, key, ok := strings.Cut(path, "/")
	if !ok {
		return "", "", false
	}

	if !storage.ValidBucket(bucket) || !storage.ValidKey(key) {
		return "", "", false
	}

	return bucket, key, true
}

func (a *localAdapter) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(a.stripBase(r.URL.Path), "/")

	bucket, key, ok := parseBucketKey(path)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPut && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// parseBucketKey validated above, so resolve cannot fail; put/get
	// recheck containment (symlink-aware) before touching the filesystem.
	full, _ := a.resolve(bucket, key)

	if r.URL.Query().Get("sig") == "" {
		a.serveUnsigned(w, r, bucket, key, full)
		return
	}

	if !a.validRequest(r, bucket, key) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodPut {
		a.put(w, r, full, a.uploadMode(bucket), r.URL.Query().Get("ct"))
		return
	}

	a.get(w, full)
}

// serveUnsigned handles requests with no signature: these are only permitted
// when the corresponding permission is public. A nil policy (not configured)
// has no public permissions, so any unsigned request is forbidden.
func (a *localAdapter) serveUnsigned(w http.ResponseWriter, r *http.Request, bucket, key, full string) {
	pol := a.PolicyFor(bucket)

	var perm storage.Perm
	if r.Method == http.MethodGet {
		perm = storage.PermRead
	} else {
		var err error

		perm, err = a.uploadPerm(bucket, key, pol)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	if !a.Configured() || !pol.Public(perm) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodPut {
		a.put(w, r, full, a.uploadMode(bucket), r.Header.Get("Content-Type"))
		return
	}

	a.get(w, full)
}

// uploadMode returns the file mode for writes based on whether the object is
// readable and writable by the public. Private reads never widen host
// permissions, so a public-write/private-read bucket stores 0600.
func (a *localAdapter) uploadMode(bucket string) os.FileMode {
	if !a.Configured() {
		return 0o600
	}

	pol := a.PolicyFor(bucket)
	if pol.Public(storage.PermRead) && (pol.Public(storage.PermWrite) || pol.Public(storage.PermUpdate)) {
		return 0o644
	}

	return 0o600
}

// validRequest verifies the query signature with a constant-time comparison
// and rejects expired URLs. The subject ("sub") is part of the signed payload
// when present, so tampering with it invalidates the signature. For private
// buckets an absent subject is rejected.
func (a *localAdapter) validRequest(r *http.Request, bucket, key string) bool {
	q := r.URL.Query()

	expires, err := strconv.ParseInt(q.Get("expires"), 10, 64)
	if err != nil || expires < time.Now().Unix() {
		return false
	}

	sub := q.Get("sub")

	if a.Configured() && sub == "" {
		if r.Method == http.MethodGet {
			if !a.PolicyFor(bucket).Public(storage.PermRead) {
				return false
			}
		} else if pol := a.PolicyFor(bucket); !pol.Public(storage.PermWrite) || !pol.Public(storage.PermUpdate) {
			return false
		}
	}

	expected := []byte(a.sign(storage.HTTPMethod(r.Method), bucket, key, q.Get("ct"), expires, storage.Subject(sub)))
	got := []byte(q.Get("sig"))

	if len(got) != len(expected) {
		return false
	}

	return subtle.ConstantTimeCompare(expected, got) == 1
}

func (a *localAdapter) ensurePutDir(full string) (string, error) {
	if !a.lexicalContainedNoEval(full) {
		return "", storage.ErrForbidden
	}

	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, a.dirModeForPut(full)); err != nil {
		return "", err
	}

	if !a.lexicallyContained(full) {
		return "", storage.ErrForbidden
	}

	return dir, nil
}

func writeBodyTemp(dir string, body io.Reader, mode os.FileMode) (string, error) {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}

	n, err := io.Copy(tmp, io.LimitReader(body, maxBody+1))
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())

		return "", err
	}

	if n > maxBody {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())

		return "", io.EOF
	}

	// Best effort: the tmpfile is freshly created and owned by us, so Chmod
	// and Close cannot fail short of disk faults (CreateTemp's 0600 default
	// is the fail-closed direction for Chmod).
	_ = tmp.Chmod(mode)
	_ = tmp.Close()

	return tmp.Name(), nil
}

// writeMetaTemp stages the content-type sidecar. It returns "" when staging
// is impossible; put treats that as "no sidecar" and still stores the object
// (callers only reach here after writeBodyTemp succeeded in the same dir, so
// CreateTemp failure is unreachable in practice and meta content is auxiliary).
func writeMetaTemp(dir, declaredType string, mode os.FileMode) string {
	mt, err := os.CreateTemp(dir, ".mtmp-*")
	if err != nil {
		return ""
	}

	// Best effort (see writeBodyTemp): higher layers fall back to extension
	// sniffing when the sidecar is absent or corrupt.
	b, _ := json.Marshal(objectMeta{ContentType: declaredType}) // struct{string}: infallible
	_, _ = mt.Write(b)
	_ = mt.Chmod(mode)
	_ = mt.Close()

	return mt.Name()
}

func (a *localAdapter) put(w http.ResponseWriter, r *http.Request, full string, mode os.FileMode, declaredType string) {
	dir, err := a.ensurePutDir(full)
	if err != nil {
		if errors.Is(err, storage.ErrForbidden) {
			http.Error(w, "forbidden", http.StatusForbidden)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}

		return
	}

	tmpName, err := writeBodyTemp(dir, r.Body, mode)
	if err != nil {
		if errors.Is(err, io.EOF) {
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}

		return
	}

	defer func() { _ = os.Remove(tmpName) }()

	metaTmp := ""

	if declaredType != "" {
		metaTmp = writeMetaTemp(dir, declaredType, mode)

		defer func() { _ = os.Remove(metaTmp) }() //nolint:gosec // metaTmp from CreateTemp
	}

	if err := os.Rename(tmpName, full); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if metaTmp != "" {
		if err := os.Rename(metaTmp, full+metaSuffix); err != nil { //nolint:gosec // metaTmp from CreateTemp
			_ = os.Remove(full)

			http.Error(w, "internal error", http.StatusInternalServerError)

			return
		}
	} else {
		_ = os.Remove(full + metaSuffix)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *localAdapter) get(w http.ResponseWriter, full string) {
	if fi, err := os.Lstat(full); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if realPath, err := filepath.EvalSymlinks(filepath.Dir(full)); err == nil && !a.realContained(realPath) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	f, err := os.Open(full) //nolint:gosec // internal path validated by resolve/lexicallyContained
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	defer func() { _ = f.Close() }()

	w.Header().Set("Content-Type", contentTypeFor(full))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func contentTypeFor(full string) string {
	f, err := os.Open(full + metaSuffix) //nolint:gosec // internal path validated by caller
	if err == nil {
		defer func() { _ = f.Close() }()

		b, err := io.ReadAll(io.LimitReader(f, 8192))
		if err == nil {
			var meta objectMeta
			if json.Unmarshal(b, &meta) == nil && meta.ContentType != "" && !isScriptable(meta.ContentType) {
				return meta.ContentType
			}
		}
	}

	mime, ok := contentTypes[extension(strings.ToLower(filepath.Ext(full)))]
	if !ok || isScriptable(string(mime)) {
		return "application/octet-stream"
	}

	return string(mime)
}

func isScriptable(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	return scriptableContentTypes[contentType(ct)]
}
