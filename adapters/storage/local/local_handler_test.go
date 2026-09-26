package local

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

var errHandlerBoom = errors.New("handler boom")

type handlerErrReader struct{}

func (handlerErrReader) Read([]byte) (int, error) { return 0, errHandlerBoom }

func newTestAdapter(t *testing.T, opts storage.Options) *localAdapter {
	t.Helper()

	if opts.Root == "" {
		opts.Root = t.TempDir()
	}

	if opts.Secret == "" {
		opts.Secret = "test-secret-key-for-handler-tests"
	}

	s, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	a, ok := s.(*localAdapter)
	if !ok {
		t.Fatalf("New returned %T, want *localAdapter", s)
	}

	return a
}

func doServe(t *testing.T, a *localAdapter, method, target string, body io.Reader, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, body)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)

	return rec
}

func signedTarget(t *testing.T, fullURL string) string {
	t.Helper()

	u, err := url.Parse(fullURL)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}

	return u.RequestURI()
}

func queryRequest(t *testing.T, method, path string, q url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	req.URL.RawQuery = q.Encode()

	return req
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}

	neg := n < 0
	if neg {
		n = -n
	}

	var buf [20]byte
	pos := len(buf)

	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}

	if neg {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}

func TestParseBucketKey(t *testing.T) {
	cases := []struct {
		path  string
		wantB string
		wantK string
		want  bool
	}{
		{"onlybucket", "", "", false},
		{"", "", "", false},
		{"bad_bucket!/key", "", "", false},
		{"bkt/", "", "", false},
		{"bkt/..", "", "", false},
		{"bkt/a/../x", "", "", false},
		{"bkt/k", "bkt", "k", true},
		{"bkt/a/b/c", "bkt", "a/b/c", true},
	}

	for _, tc := range cases {
		b, k, ok := parseBucketKey(tc.path)
		if ok != tc.want || b != tc.wantB || k != tc.wantK {
			t.Errorf("parseBucketKey(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tc.path, b, k, ok, tc.wantB, tc.wantK, tc.want)
		}
	}
}

func TestStripBase(t *testing.T) {
	a := newTestAdapter(t, storage.Options{URLBase: "https://cdn.example.com/base"})

	if got := a.stripBase("/base/bkt/k"); got != "/bkt/k" {
		t.Errorf("stripBase prefixed = %q, want %q", got, "/bkt/k")
	}

	if got := a.stripBase("/other/bkt/k"); got != "/other/bkt/k" {
		t.Errorf("stripBase non-prefix = %q, want unchanged", got)
	}

	plain := newTestAdapter(t, storage.Options{})
	if got := plain.stripBase("/bkt/k"); got != "/bkt/k" {
		t.Errorf("stripBase empty urlBase = %q, want unchanged", got)
	}

	nopath := newTestAdapter(t, storage.Options{URLBase: "https://cdn.example.com"})
	if got := nopath.stripBase("/bkt/k"); got != "/bkt/k" {
		t.Errorf("stripBase empty base path = %q, want unchanged", got)
	}
}

func TestServeRejects(t *testing.T) {
	a := newTestAdapter(t, storage.Options{})

	if rec := doServe(t, a, http.MethodGet, "/onlybucket", nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("GET no-slash = %d, want 403", rec.Code)
	}

	if rec := doServe(t, a, http.MethodGet, "/bad_bucket!/k", nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("GET bad bucket = %d, want 403", rec.Code)
	}

	if rec := doServe(t, a, http.MethodPost, "/bkt/k", nil, nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", rec.Code)
	}

	expires := time.Now().Add(time.Hour).Unix()
	q := url.Values{}
	q.Set("expires", itoa(expires))
	q.Set("sig", "short")

	req := queryRequest(t, http.MethodGet, "/bkt/k", q)
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("GET bad sig = %d, want 403", rec.Code)
	}
}

func TestServeSignedPutGet(t *testing.T) {
	ctx := t.Context()
	a := newTestAdapter(t, storage.Options{})

	pu, err := a.PresignUpload(ctx, "bkt", "obj.txt", "text/plain", time.Hour)
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}

	rec := doServe(t, a, http.MethodPut, signedTarget(t, pu.URL), strings.NewReader("hello"), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT signed = %d, want 204", rec.Code)
	}

	raw, err2 := os.ReadFile(filepath.Join(a.root, "bkt", "obj.txt"))
	if err2 != nil || string(raw) != "hello" {
		t.Fatalf("stored body = %q, err %v; want %q", string(raw), err2, "hello")
	}

	pd, err3 := a.PresignDownload(ctx, "bkt", "obj.txt", time.Hour)
	if err3 != nil {
		t.Fatalf("PresignDownload: %v", err3)
	}

	rec2 := doServe(t, a, http.MethodGet, signedTarget(t, pd.URL), nil, nil)
	if rec2.Code != http.StatusOK || rec2.Body.String() != "hello" {
		t.Errorf("GET signed = %d body %q, want 200 hello", rec2.Code, rec2.Body.String())
	}

	if ct := rec2.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("GET Content-Type = %q, want text/plain from meta", ct)
	}
}

func TestServeSignedWithURLBase(t *testing.T) {
	ctx := t.Context()
	a := newTestAdapter(t, storage.Options{URLBase: "https://cdn.example.com/base"})

	pu, err := a.PresignUpload(ctx, "bkt", "a/b.bin", "", time.Hour)
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}

	target := signedTarget(t, pu.URL)
	if !strings.HasPrefix(target, "/base/bkt/a/b.bin") {
		t.Fatalf("target = %q, want /base/ prefix", target)
	}

	if rec := doServe(t, a, http.MethodPut, target, strings.NewReader("based"), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT base = %d, want 204", rec.Code)
	}

	pd, err2 := a.PresignDownload(ctx, "bkt", "a/b.bin", time.Hour)
	if err2 != nil {
		t.Fatalf("PresignDownload: %v", err2)
	}

	rec := doServe(t, a, http.MethodGet, signedTarget(t, pd.URL), nil, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "based" {
		t.Errorf("GET base = %d body %q, want 200 based", rec.Code, rec.Body.String())
	}
}

func publicPolicy() *storage.PolicyConfig {
	return &storage.PolicyConfig{
		Default: &storage.Policy{
			Read:   storage.Rule{Public: true},
			Write:  storage.Rule{Public: true},
			Update: storage.Rule{Public: true},
		},
	}
}

func TestServeUnsigned(t *testing.T) {
	a := newTestAdapter(t, storage.Options{Policy: publicPolicy()})

	if err := os.MkdirAll(filepath.Join(a.root, "pub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(a.root, "pub", "obj"), []byte("pubdata"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec := doServe(t, a, http.MethodGet, "/pub/obj", nil, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "pubdata" {
		t.Errorf("GET public = %d body %q, want 200 pubdata", rec.Code, rec.Body.String())
	}

	if rec := doServe(t, a, http.MethodGet, "/pub/missing", nil, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET public missing = %d, want 404", rec.Code)
	}

	rec2 := doServe(t, a, http.MethodPut, "/pub/new.txt", strings.NewReader("hello"),
		map[string]string{"Content-Type": "text/plain"})
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("PUT public = %d, want 204", rec2.Code)
	}

	rec3 := doServe(t, a, http.MethodGet, "/pub/new.txt", nil, nil)
	if rec3.Code != http.StatusOK || rec3.Body.String() != "hello" {
		t.Errorf("GET after PUT = %d body %q, want 200 hello", rec3.Code, rec3.Body.String())
	}

	if ct := rec3.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("GET ct = %q, want text/plain from header-declared meta", ct)
	}

	priv := newTestAdapter(t, storage.Options{
		Policy: &storage.PolicyConfig{
			Default: &storage.Policy{Read: storage.Rule{Allow: []storage.Subject{"alice"}}},
		},
	})

	if rec := doServe(t, priv, http.MethodGet, "/bkt/k", nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("GET private = %d, want 403", rec.Code)
	}

	if rec := doServe(t, priv, http.MethodPut, "/bkt/k", strings.NewReader("x"), nil); rec.Code != http.StatusForbidden {
		t.Errorf("PUT private = %d, want 403", rec.Code)
	}

	legacy := newTestAdapter(t, storage.Options{})
	if rec := doServe(t, legacy, http.MethodGet, "/bkt/k", nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("GET unconfigured = %d, want 403", rec.Code)
	}

	if rec := doServe(t, legacy, http.MethodPut, "/bkt/k", strings.NewReader("x"), nil); rec.Code != http.StatusForbidden {
		t.Errorf("PUT unconfigured = %d, want 403", rec.Code)
	}
}

func TestServeUnsignedUploadPermError(t *testing.T) {
	a := newTestAdapter(t, storage.Options{
		Policy: &storage.PolicyConfig{
			Default: &storage.Policy{
				Write: storage.Rule{Public: true},
			},
		},
	})

	bdir := filepath.Join(a.root, "bkt")
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.Chmod(bdir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	defer func() { _ = os.Chmod(bdir, 0o700) }()

	if rec := doServe(t, a, http.MethodPut, "/bkt/k", strings.NewReader("x"), nil); rec.Code != http.StatusInternalServerError {
		t.Errorf("PUT stat-error = %d, want 500", rec.Code)
	}
}

func TestValidRequest(t *testing.T) {
	cfg := &storage.PolicyConfig{
		Default: &storage.Policy{Read: storage.Rule{Allow: []storage.Subject{"alice"}}},
		Buckets: map[storage.BucketName]storage.Policy{
			"pub":  {Read: storage.Rule{Public: true}},
			"wu":   {Write: storage.Rule{Public: true}, Update: storage.Rule{Public: true}},
			"wpub": {Write: storage.Rule{Public: true}},
		},
	}
	a := newTestAdapter(t, storage.Options{Policy: cfg})
	open := newTestAdapter(t, storage.Options{})

	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()

	q := url.Values{}
	q.Set("expires", "abc")
	q.Set("sig", "x")

	if open.validRequest(queryRequest(t, http.MethodGet, "/b/k", q), "b", "k") {
		t.Errorf("bad expires accepted")
	}

	pastSig := open.sign(http.MethodGet, "b", "k", "", past, "")
	q2 := url.Values{}
	q2.Set("expires", itoa(past))
	q2.Set("sig", pastSig)

	if open.validRequest(queryRequest(t, http.MethodGet, "/b/k", q2), "b", "k") {
		t.Errorf("expired accepted")
	}

	privSig := a.sign(http.MethodGet, "any", "k", "", future, "")
	q3 := url.Values{}
	q3.Set("expires", itoa(future))
	q3.Set("sig", privSig)

	if a.validRequest(queryRequest(t, http.MethodGet, "/any/k", q3), "any", "k") {
		t.Errorf("absent-sub private GET accepted")
	}

	pubSig := a.sign(http.MethodGet, "pub", "k", "", future, "")
	q4 := url.Values{}
	q4.Set("expires", itoa(future))
	q4.Set("sig", pubSig)

	if !a.validRequest(queryRequest(t, http.MethodGet, "/pub/k", q4), "pub", "k") {
		t.Errorf("absent-sub public GET rejected")
	}

	putSig := a.sign(http.MethodPut, "wpub", "k", "", future, "")
	q5 := url.Values{}
	q5.Set("expires", itoa(future))
	q5.Set("sig", putSig)

	if a.validRequest(queryRequest(t, http.MethodPut, "/wpub/k", q5), "wpub", "k") {
		t.Errorf("write-only-public PUT without sub accepted")
	}

	wuSig := a.sign(http.MethodPut, "wu", "k", "", future, "")
	q6 := url.Values{}
	q6.Set("expires", itoa(future))
	q6.Set("sig", wuSig)

	if !a.validRequest(queryRequest(t, http.MethodPut, "/wu/k", q6), "wu", "k") {
		t.Errorf("write+update-public PUT without sub rejected")
	}

	openSig := open.sign(http.MethodPut, "b", "k", "text/plain", future, "")
	q7 := url.Values{}
	q7.Set("ct", "text/plain")
	q7.Set("expires", itoa(future))
	q7.Set("sig", openSig)

	if !open.validRequest(queryRequest(t, http.MethodPut, "/b/k", q7), "b", "k") {
		t.Errorf("unconfigured PUT rejected")
	}

	q8 := url.Values{}
	q8.Set("expires", itoa(future))
	q8.Set("sig", "deadbeef")

	if open.validRequest(queryRequest(t, http.MethodGet, "/b/k", q8), "b", "k") {
		t.Errorf("short sig accepted")
	}

	good := open.sign(http.MethodGet, "b", "k", "", future, "")
	last := good[len(good)-1]
	flip := byte('a')
	if last == 'a' {
		flip = 'b'
	}

	q9 := url.Values{}
	q9.Set("expires", itoa(future))
	q9.Set("sig", good[:len(good)-1]+string(flip))

	if open.validRequest(queryRequest(t, http.MethodGet, "/b/k", q9), "b", "k") {
		t.Errorf("wrong sig accepted")
	}

	q10 := url.Values{}
	q10.Set("expires", itoa(future))
	q10.Set("sig", good)

	if !open.validRequest(queryRequest(t, http.MethodGet, "/b/k", q10), "b", "k") {
		t.Errorf("correct sig rejected")
	}

	aliceSig := a.sign(http.MethodGet, "any", "k", "", future, "alice")
	q11 := url.Values{}
	q11.Set("expires", itoa(future))
	q11.Set("sig", aliceSig)
	q11.Set("sub", "alice")

	if !a.validRequest(queryRequest(t, http.MethodGet, "/any/k", q11), "any", "k") {
		t.Errorf("private GET with sub rejected")
	}
}

func TestUploadMode(t *testing.T) {
	open := newTestAdapter(t, storage.Options{})
	if m := open.uploadMode("b"); m != 0o600 {
		t.Errorf("unconfigured mode = %o, want 600", m)
	}

	pub := newTestAdapter(t, storage.Options{Policy: publicPolicy()})
	if m := pub.uploadMode("b"); m != 0o644 {
		t.Errorf("public mode = %o, want 644", m)
	}

	readOnly := newTestAdapter(t, storage.Options{
		Policy: &storage.PolicyConfig{
			Default: &storage.Policy{Read: storage.Rule{Public: true}},
		},
	})
	if m := readOnly.uploadMode("b"); m != 0o600 {
		t.Errorf("read-public-only mode = %o, want 600", m)
	}

	writeOnly := newTestAdapter(t, storage.Options{
		Policy: &storage.PolicyConfig{
			Default: &storage.Policy{Write: storage.Rule{Public: true}},
		},
	})
	if m := writeOnly.uploadMode("b"); m != 0o600 {
		t.Errorf("write-public-only mode = %o, want 600", m)
	}

	readUpdate := newTestAdapter(t, storage.Options{
		Policy: &storage.PolicyConfig{
			Default: &storage.Policy{
				Read:   storage.Rule{Public: true},
				Update: storage.Rule{Public: true},
			},
		},
	})
	if m := readUpdate.uploadMode("b"); m != 0o644 {
		t.Errorf("read+update-public mode = %o, want 644", m)
	}
}

func TestEnsurePutDir(t *testing.T) {
	a := newTestAdapter(t, storage.Options{})

	dir, err := a.ensurePutDir(filepath.Join(a.root, "bkt", "a", "obj"))
	if err != nil {
		t.Fatalf("ensurePutDir: %v", err)
	}

	if st, err2 := os.Stat(dir); err2 != nil || !st.IsDir() {
		t.Fatalf("dir missing: %v", err2)
	}

	if _, err := a.ensurePutDir(filepath.Join(a.root, "..", "evil")); !errors.Is(err, storage.ErrForbidden) {
		t.Errorf("outside root err = %v, want ErrForbidden", err)
	}

	locked := filepath.Join(a.root, "locked")
	if err := os.MkdirAll(locked, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	defer func() { _ = os.Chmod(locked, 0o700) }()

	if _, err := a.ensurePutDir(filepath.Join(locked, "child", "obj")); err == nil || errors.Is(err, storage.ErrForbidden) {
		t.Errorf("mkdir-error err = %v, want non-forbidden error", err)
	}

	outside := t.TempDir()
	link := filepath.Join(a.root, "linkbkt")

	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := a.ensurePutDir(filepath.Join(link, "obj")); !errors.Is(err, storage.ErrForbidden) {
		t.Errorf("symlink escape err = %v, want ErrForbidden", err)
	}
}

func TestWriteBodyTemp(t *testing.T) {
	dir := t.TempDir()

	name, err := writeBodyTemp(dir, strings.NewReader("data"), 0o600)
	if err != nil {
		t.Fatalf("writeBodyTemp: %v", err)
	}

	raw, err2 := os.ReadFile(name)
	if err2 != nil || string(raw) != "data" {
		t.Fatalf("body = %q err %v", string(raw), err2)
	}

	st, err3 := os.Stat(name)
	if err3 != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v err %v", st.Mode(), err3)
	}

	if _, err := writeBodyTemp(filepath.Join(dir, "nodir"), strings.NewReader("x"), 0o600); err == nil {
		t.Errorf("missing dir accepted")
	}

	if _, err := writeBodyTemp(dir, handlerErrReader{}, 0o600); !errors.Is(err, errHandlerBoom) {
		t.Errorf("copy err = %v, want boom", err)
	}

	old := maxBody
	maxBody = 4

	defer func() { maxBody = old }()

	if _, err := writeBodyTemp(dir, strings.NewReader("toolarge"), 0o600); !errors.Is(err, io.EOF) {
		t.Errorf("oversize err = %v, want EOF", err)
	}
}

func TestWriteMetaTemp(t *testing.T) {
	dir := t.TempDir()

	name := writeMetaTemp(dir, "text/plain", 0o600)
	if name == "" {
		t.Fatal("writeMetaTemp returned empty name")
	}

	raw, err2 := os.ReadFile(name)
	if err2 != nil {
		t.Fatalf("read: %v", err2)
	}

	var m objectMeta
	if err3 := json.Unmarshal(raw, &m); err3 != nil || m.ContentType != "text/plain" {
		t.Fatalf("meta = %q err %v", string(raw), err3)
	}

	if got := writeMetaTemp(filepath.Join(dir, "nodir"), "text/plain", 0o600); got != "" {
		t.Errorf("missing dir accepted: %q", got)
	}
}

func putDirect(t *testing.T, a *localAdapter, full, body, ct string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/x", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.put(rec, req, full, 0o600, ct)

	return rec
}

func putDirectReader(t *testing.T, a *localAdapter, full string, body io.Reader, ct string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/x", body)
	rec := httptest.NewRecorder()
	a.put(rec, req, full, 0o600, ct)

	return rec
}

func TestPutBranches(t *testing.T) {
	a := newTestAdapter(t, storage.Options{})

	if rec := putDirect(t, a, filepath.Join(a.root, "..", "evil"), "x", ""); rec.Code != http.StatusForbidden {
		t.Errorf("outside root = %d, want 403", rec.Code)
	}

	locked := filepath.Join(a.root, "locked")
	if err := os.MkdirAll(locked, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	defer func() { _ = os.Chmod(locked, 0o700) }()

	if rec := putDirect(t, a, filepath.Join(locked, "child", "f"), "x", ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("mkdir fail = %d, want 500", rec.Code)
	}

	full := filepath.Join(a.root, "bkt", "f")
	if rec := putDirectReader(t, a, full, handlerErrReader{}, ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("body err = %d, want 500", rec.Code)
	}

	old := maxBody
	maxBody = 4

	if rec := putDirect(t, a, filepath.Join(a.root, "bkt", "big"), "toolarge", ""); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize = %d, want 413", rec.Code)
	}

	maxBody = old

	dirTarget := filepath.Join(a.root, "bkt", "dirtarget")
	if err := os.MkdirAll(dirTarget, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dirTarget, "child"), []byte("c"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if rec := putDirect(t, a, dirTarget, "x", ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("rename onto dir = %d, want 500", rec.Code)
	}

	metaBlock := filepath.Join(a.root, "bkt", "mobj")
	if err := os.MkdirAll(metaBlock+metaSuffix, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(metaBlock+metaSuffix, "child"), []byte("c"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if rec := putDirect(t, a, metaBlock, "x", "text/plain"); rec.Code != http.StatusInternalServerError {
		t.Errorf("meta rename fail = %d, want 500", rec.Code)
	}

	if _, err := os.Stat(metaBlock); !os.IsNotExist(err) {
		t.Errorf("body left after meta failure")
	}

	stale := filepath.Join(a.root, "bkt", "stale.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(stale, []byte("old"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := os.WriteFile(stale+metaSuffix, []byte(`{"contentType":"image/png"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if rec := putDirect(t, a, stale, "new", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("stale-meta put = %d, want 204", rec.Code)
	}

	if _, err := os.Stat(stale + metaSuffix); !os.IsNotExist(err) {
		t.Errorf("stale meta not removed")
	}

	raw, err := os.ReadFile(stale)
	if err != nil || string(raw) != "new" {
		t.Fatalf("body = %q err %v", string(raw), err)
	}

	typed := filepath.Join(a.root, "bkt", "typed.bin")
	if rec := putDirect(t, a, typed, "pix", "image/png"); rec.Code != http.StatusNoContent {
		t.Fatalf("typed put = %d, want 204", rec.Code)
	}

	mraw, err2 := os.ReadFile(typed + metaSuffix)
	if err2 != nil {
		t.Fatalf("meta read: %v", err2)
	}

	var m objectMeta
	if err3 := json.Unmarshal(mraw, &m); err3 != nil || m.ContentType != "image/png" {
		t.Fatalf("meta = %q err %v", string(mraw), err3)
	}

	st, err4 := os.Stat(typed)
	if err4 != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v err %v", st.Mode(), err4)
	}
}

func TestGetBranches(t *testing.T) {
	a := newTestAdapter(t, storage.Options{})

	outside := t.TempDir()
	secretFile := filepath.Join(outside, "secret")
	if err := os.WriteFile(secretFile, []byte("s"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	bdir := filepath.Join(a.root, "bkt")
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	link := filepath.Join(bdir, "link")
	if err := os.Symlink(secretFile, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rec := httptest.NewRecorder()
	a.get(rec, link)

	if rec.Code != http.StatusForbidden {
		t.Errorf("symlink file = %d, want 403", rec.Code)
	}

	extDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(extDir, "obj"), []byte("ext"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	escLink := filepath.Join(a.root, "esc")
	if err := os.Symlink(extDir, escLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rec2 := httptest.NewRecorder()
	a.get(rec2, filepath.Join(escLink, "obj"))

	if rec2.Code != http.StatusForbidden {
		t.Errorf("dir escape = %d, want 403", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	a.get(rec3, filepath.Join(a.root, "bkt", "missing"))

	if rec3.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec3.Code)
	}

	gone := filepath.Join(a.root, "gone", "f")
	rec4 := httptest.NewRecorder()
	a.get(rec4, gone)

	if rec4.Code != http.StatusNotFound {
		t.Errorf("gone dir = %d, want 404", rec4.Code)
	}

	denied := filepath.Join(bdir, "denied")
	if err := os.WriteFile(denied, []byte("x"), 0o000); err != nil {
		t.Fatalf("write: %v", err)
	}

	defer func() { _ = os.Chmod(denied, 0o600) }()

	rec5 := httptest.NewRecorder()
	a.get(rec5, denied)

	if rec5.Code != http.StatusInternalServerError {
		t.Errorf("denied = %d, want 500", rec5.Code)
	}

	txt := filepath.Join(bdir, "a.txt")
	if err := os.WriteFile(txt, []byte("txtbody"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec6 := httptest.NewRecorder()
	a.get(rec6, txt)

	if rec6.Code != http.StatusOK || rec6.Body.String() != "txtbody" {
		t.Fatalf("txt get = %d body %q, want 200 txtbody", rec6.Code, rec6.Body.String())
	}

	if ct := rec6.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("txt ct = %q, want text/plain", ct)
	}

	if v := rec6.Header().Get("X-Content-Type-Options"); v != "nosniff" {
		t.Errorf("nosniff = %q, want nosniff", v)
	}

	binObj := filepath.Join(bdir, "a.bin")
	if err := os.WriteFile(binObj, []byte("bindata"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec7 := httptest.NewRecorder()
	a.get(rec7, binObj)

	if ct := rec7.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("bin ct = %q, want octet-stream", ct)
	}

	over := filepath.Join(bdir, "over.txt")
	if err := os.WriteFile(over, []byte("pix"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := os.WriteFile(over+metaSuffix, []byte(`{"contentType":"image/png"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec8 := httptest.NewRecorder()
	a.get(rec8, over)

	if ct := rec8.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("meta ct = %q, want image/png", ct)
	}
}

func TestContentTypeFor(t *testing.T) {
	dir := t.TempDir()

	write := func(name, data string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		return p
	}

	p := write("img.bin", "x")
	if err := os.WriteFile(p+metaSuffix, []byte(`{"contentType":"image/png"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := contentTypeFor(p); got != "image/png" {
		t.Errorf("meta ct = %q, want image/png", got)
	}

	corrupt := write("c.txt", "x")
	if err := os.WriteFile(corrupt+metaSuffix, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := contentTypeFor(corrupt); got != "text/plain" {
		t.Errorf("corrupt meta = %q, want text/plain", got)
	}

	script := write("s.txt", "x")
	if err := os.WriteFile(script+metaSuffix, []byte(`{"contentType":"text/html"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := contentTypeFor(script); got != "text/plain" {
		t.Errorf("scriptable meta = %q, want text/plain", got)
	}

	empty := write("e.txt", "x")
	if err := os.WriteFile(empty+metaSuffix, []byte(`{"contentType":""}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := contentTypeFor(empty); got != "text/plain" {
		t.Errorf("empty meta = %q, want text/plain", got)
	}

	if got := contentTypeFor(write("k.png", "x")); got != "image/png" {
		t.Errorf("ext = %q, want image/png", got)
	}

	if got := contentTypeFor(write("K.PNG", "x")); got != "image/png" {
		t.Errorf("upper ext = %q, want image/png", got)
	}

	if got := contentTypeFor(write("k.unknownext", "x")); got != "application/octet-stream" {
		t.Errorf("unknown ext = %q, want octet-stream", got)
	}

	if got := contentTypeFor(write("k.html", "x")); got != "application/octet-stream" {
		t.Errorf("html ext = %q, want octet-stream", got)
	}
}

func TestIsScriptable(t *testing.T) {
	if !isScriptable("text/html") {
		t.Errorf("text/html not scriptable")
	}

	if !isScriptable("Text/HTML; charset=utf-8") {
		t.Errorf("html+charset not scriptable")
	}

	if !isScriptable("image/svg+xml") {
		t.Errorf("svg not scriptable")
	}

	if isScriptable("text/plain") {
		t.Errorf("text/plain scriptable")
	}

	if isScriptable("application/octet-stream") {
		t.Errorf("octet-stream scriptable")
	}

	if isScriptable("image/png") {
		t.Errorf("png scriptable")
	}
}

func TestSign(t *testing.T) {
	a := newTestAdapter(t, storage.Options{})

	exp := time.Now().Add(time.Hour).Unix()
	s1 := a.sign(http.MethodGet, "b", "k", "", exp, "alice")
	s2 := a.sign(http.MethodGet, "b", "k", "", exp, "alice")

	if s1 != s2 {
		t.Errorf("sign not deterministic")
	}

	if s3 := a.sign(http.MethodGet, "b", "k", "", exp, "bob"); s3 == s1 {
		t.Errorf("sub not bound")
	}

	if s4 := a.sign(http.MethodPut, "b", "k", "", exp, "alice"); s4 == s1 {
		t.Errorf("method not bound")
	}

	if len(s1) != 64 {
		t.Errorf("sig len = %d, want 64", len(s1))
	}
}

func TestURLFor(t *testing.T) {
	a := newTestAdapter(t, storage.Options{URLBase: "https://cdn.example.com/base"})
	exp := time.Now().Add(time.Hour).Unix()

	u, err := url.Parse(a.urlFor("b", "k", "text/plain", exp, "sigv", "alice"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	q := u.Query()
	if q.Get("ct") != "text/plain" || q.Get("expires") != itoa(exp) || q.Get("sig") != "sigv" || q.Get("sub") != "alice" {
		t.Errorf("query = %v, want ct/expires/sig/sub", q)
	}

	if !strings.HasPrefix(u.Path, "/base/b/k") {
		t.Errorf("path = %q, want /base/ prefix", u.Path)
	}

	u2, err2 := url.Parse(a.urlFor("b", "k", "", exp, "sigv", ""))
	if err2 != nil {
		t.Fatalf("parse: %v", err2)
	}

	q2 := u2.Query()
	if q2.Get("ct") != "" || q2.Get("sub") != "" {
		t.Errorf("empty ct/sub present: %v", q2)
	}

	if q2.Get("expires") != itoa(exp) || q2.Get("sig") != "sigv" {
		t.Errorf("query = %v, want expires/sig", q2)
	}
}
