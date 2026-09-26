package s3opts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/core/storage"
)

var errBoom = errors.New("boom")

const copyResultXML = `<CopyObjectResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">` +
	`<LastModified>2024-01-01T00:00:00Z</LastModified><ETag>"abc"</ETag></CopyObjectResult>`

type stubTransport struct {
	calls int
	fn    func(req *http.Request) (int, string, error)
}

func (s *stubTransport) Do(req *http.Request) (*http.Response, error) {
	s.calls++
	status, body, err := s.fn(req)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode:    status,
		Header:        http.Header{},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

func okStub(_ *http.Request) (int, string, error) {
	return http.StatusOK, "", nil
}

func notFoundStub(_ *http.Request) (int, string, error) {
	return http.StatusNotFound, "", nil
}

func forbiddenStub(_ *http.Request) (int, string, error) {
	return http.StatusForbidden, "", nil
}

func boomStub(_ *http.Request) (int, string, error) {
	return 0, "", errBoom
}

func copyOKStub(_ *http.Request) (int, string, error) {
	return http.StatusOK, copyResultXML, nil
}

func deleteOKStub(_ *http.Request) (int, string, error) {
	return http.StatusNoContent, "", nil
}

func newTestCore(
	ctx context.Context,
	t *testing.T,
	fn stubFn,
	store storage.PolicyStore,
	staticURL func(string, string) (string, error),
) (*Core, *stubTransport) {
	t.Helper()
	stub := &stubTransport{fn: fn}
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("ak", "sk", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.HTTPClient = stub
		o.Retryer = aws.NopRetryer{}
	})
	return New("test", "us-east-1", "", client, s3.NewPresignClient(client), store, staticURL), stub
}

func storeWith(t *testing.T, def *storage.Policy, buckets map[storage.BucketName]storage.Policy) storage.PolicyStore {
	t.Helper()
	var store storage.PolicyStore
	if err := store.ResolveFromConfig(&storage.PolicyConfig{Default: def, Buckets: buckets}); err != nil {
		t.Fatal(err)
	}
	return store
}

func fullAccess() storage.Policy {
	r := storage.Rule{Allow: []storage.Subject{"alice"}}
	return storage.Policy{Read: r, Write: r, Update: r, Delete: r}
}

func publicUploadPolicy() *storage.Policy {
	return &storage.Policy{
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}
}

func mustPresignUpload(ctx context.Context, t *testing.T, c *Core, key string) storage.PresignedURL {
	t.Helper()
	out, err := c.PresignUpload(ctx, "bkt", key, "text/plain", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustPresignDownload(ctx context.Context, t *testing.T, c *Core, bucket, key string) storage.PresignedURL {
	t.Helper()
	out, err := c.PresignDownload(ctx, bucket, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNewStaticURLDefault(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	if c.prefix != "test" {
		t.Fatalf("prefix = %q, want %q", c.prefix, "test")
	}
	if c.staticURL == nil {
		t.Fatal("staticURL = nil, want default")
	}
	got, err := c.staticURL("bkt", "a/b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "bkt.s3.us-east-1.amazonaws.com") {
		t.Fatalf("staticURL = %q, want amazonaws host", got)
	}
	if !strings.HasSuffix(got, "/a/b") {
		t.Fatalf("staticURL = %q, want /a/b suffix", got)
	}
}

func TestNewStaticURLCustom(t *testing.T) {
	ctx := t.Context()
	custom := func(_, _ string) (string, error) { return "https://cdn.example/x", nil }
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, custom)
	got, err := c.staticURL("bkt", "k")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://cdn.example/x" {
		t.Fatalf("staticURL = %q, want custom URL", got)
	}
}

func TestNewCoreClient(t *testing.T) {
	ctx := t.Context()
	if _, _, err := NewCoreClient(ctx, "p", "us-east-1", "://bad", "", "ak", "sk"); err == nil {
		t.Fatal("NewCoreClient(bad endpoint) = nil, want error")
	}
	if _, _, err := NewCoreClient(ctx, "p", "us-east-1", "", "://bad", "ak", "sk"); err == nil {
		t.Fatal("NewCoreClient(bad url_base) = nil, want error")
	}
	plain, presigner, err := NewCoreClient(ctx, "p", "us-east-1", "", "", "ak", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if presigner == nil {
		t.Fatal("presigner = nil")
	}
	if plain.Options().BaseEndpoint != nil {
		t.Fatalf("BaseEndpoint = %q, want nil", *plain.Options().BaseEndpoint)
	}
	if plain.Options().Region != "us-east-1" {
		t.Fatalf("Region = %q, want us-east-1", plain.Options().Region)
	}
	ep, _, err := NewCoreClient(ctx, "p", "eu-west-1", "http://localhost:9000", "", "ak", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if ep.Options().BaseEndpoint == nil || *ep.Options().BaseEndpoint != "http://localhost:9000" {
		t.Fatalf("BaseEndpoint = %v, want http://localhost:9000", ep.Options().BaseEndpoint)
	}
	if !ep.Options().UsePathStyle {
		t.Fatal("UsePathStyle = false, want true")
	}
	if ep.Options().Region != "eu-west-1" {
		t.Fatalf("Region = %q, want eu-west-1", ep.Options().Region)
	}
	ub, _, err := NewCoreClient(ctx, "p", "eu-west-1", "", "https://cdn.example", "ak", "sk")
	if err != nil {
		t.Fatal(err)
	}
	if ub.Options().BaseEndpoint == nil || *ub.Options().BaseEndpoint != "https://cdn.example" {
		t.Fatalf("BaseEndpoint = %v, want https://cdn.example", ub.Options().BaseEndpoint)
	}
}

func TestNewCoreClientLoadConfigError(t *testing.T) {
	ctx := t.Context()
	t.Setenv("AWS_MAX_ATTEMPTS", "bogus")
	if _, _, err := NewCoreClient(ctx, "p", "us-east-1", "", "", "ak", "sk"); err == nil {
		t.Fatal("NewCoreClient(bad env) = nil, want load config error")
	}
}

func TestIsPublicUpload(t *testing.T) {
	pub := storage.Policy{Write: storage.Rule{Public: true}, Update: storage.Rule{Public: true}}
	if !isPublicUpload(pub) {
		t.Fatal("isPublicUpload(public) = false, want true")
	}
	half := storage.Policy{Write: storage.Rule{Public: true}}
	if isPublicUpload(half) {
		t.Fatal("isPublicUpload(write-only) = true, want false")
	}
	if isPublicUpload(storage.Policy{}) {
		t.Fatal("isPublicUpload(zero) = true, want false")
	}
}

func TestUploadPerm(t *testing.T) {
	ctx := t.Context()
	pub := storage.Policy{Write: storage.Rule{Public: true}, Update: storage.Rule{Public: true}}
	split := storage.Policy{
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"bob"}},
	}
	c, _ := newTestCore(ctx, t, boomStub, storage.PolicyStore{}, nil)
	if got := c.uploadPerm(ctx, "b", "k", pub); got != storage.PermWrite {
		t.Fatalf("uploadPerm(public) = %q, want write", got)
	}
	if got := c.uploadPerm(ctx, "b", "k", storage.Policy{}); got != storage.PermWrite {
		t.Fatalf("uploadPerm(private, no check) = %q, want write", got)
	}
	if got := c.uploadPerm(ctx, "b", "k", split); got != storage.PermWrite {
		t.Fatalf("uploadPerm(split, exists err) = %q, want write", got)
	}
	ce, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	if got := ce.uploadPerm(ctx, "b", "k", split); got != storage.PermUpdate {
		t.Fatalf("uploadPerm(split, exists) = %q, want update", got)
	}
	cm, _ := newTestCore(ctx, t, notFoundStub, storage.PolicyStore{}, nil)
	if got := cm.uploadPerm(ctx, "b", "k", split); got != storage.PermWrite {
		t.Fatalf("uploadPerm(split, missing) = %q, want write", got)
	}
}

func TestPresignUploadValidation(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	if _, err := c.PresignUpload(ctx, "bad bucket!", "k", "text/plain", time.Minute); err == nil {
		t.Fatal("PresignUpload(bad bucket) = nil, want error")
	}
	if _, err := c.PresignUpload(ctx, "bkt", "", "text/plain", time.Minute); err == nil {
		t.Fatal("PresignUpload(bad key) = nil, want error")
	}
	if _, err := c.PresignUpload(ctx, "bkt", "k", "text/plain", 0); err == nil {
		t.Fatal("PresignUpload(zero ttl) = nil, want error")
	}
	if _, err := c.PresignUpload(ctx, "bkt", "k", "text/plain", 8*24*time.Hour); err == nil {
		t.Fatal("PresignUpload(huge ttl) = nil, want error")
	}
}

func TestPresignUploadUnconfigured(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	out := mustPresignUpload(ctx, t, c, "dir/k")
	if out.Method != http.MethodPut {
		t.Fatalf("Method = %q, want PUT", out.Method)
	}
	for _, want := range []string{"bkt", "dir/k", "X-Amz-Signature"} {
		if !strings.Contains(out.URL, want) {
			t.Fatalf("URL %q missing %q", out.URL, want)
		}
	}
}

func TestPresignUploadPublic(t *testing.T) {
	ctx := t.Context()
	store := storeWith(t, publicUploadPolicy(), nil)
	c, stub := newTestCore(ctx, t, boomStub, store, nil)
	out := mustPresignUpload(ctx, t, c, "k")
	if out.Method != http.MethodPut {
		t.Fatalf("Method = %q, want PUT", out.Method)
	}
	if !strings.Contains(out.URL, "bkt.s3.us-east-1.amazonaws.com") {
		t.Fatalf("URL = %q, want static amazonaws URL", out.URL)
	}
	if stub.calls != 0 {
		t.Fatalf("calls = %d, want 0 (static URL needs no HTTP)", stub.calls)
	}
}

func TestPresignUploadPublicCustomStatic(t *testing.T) {
	ctx := t.Context()
	store := storeWith(t, publicUploadPolicy(), nil)
	custom := func(_, _ string) (string, error) { return "https://cdn.example/u", nil }
	c, _ := newTestCore(ctx, t, okStub, store, custom)
	out := mustPresignUpload(ctx, t, c, "k")
	if out.URL != "https://cdn.example/u" {
		t.Fatalf("URL = %q, want custom static URL", out.URL)
	}
}

func TestPresignUploadStaticError(t *testing.T) {
	ctx := t.Context()
	store := storeWith(t, publicUploadPolicy(), nil)
	failing := func(_, _ string) (string, error) { return "", errBoom }
	c, _ := newTestCore(ctx, t, okStub, store, failing)
	if _, err := c.PresignUpload(ctx, "bkt", "k", "text/plain", time.Minute); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestPresignUploadPrivate(t *testing.T) {
	ctx := t.Context()
	pol := &storage.Policy{
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"alice"}},
	}
	c, _ := newTestCore(ctx, t, okStub, storeWith(t, pol, nil), nil)
	if _, err := c.PresignUpload(ctx, "bkt", "k", "text/plain", time.Minute); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	out := mustPresignUpload(storage.WithSubject(ctx, "alice"), t, c, "k")
	if out.Method != http.MethodPut {
		t.Fatalf("Method = %q, want PUT", out.Method)
	}
	if !strings.Contains(out.URL, "X-Amz-Signature") {
		t.Fatalf("URL = %q, want presigned URL", out.URL)
	}
}

func TestPresignUploadExistCheck(t *testing.T) {
	ctx := t.Context()
	split := &storage.Policy{
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"bob"}},
	}
	ce, _ := newTestCore(ctx, t, okStub, storeWith(t, split, nil), nil)
	out := mustPresignUpload(storage.WithSubject(ctx, "bob"), t, ce, "k")
	if !strings.Contains(out.URL, "X-Amz-Signature") {
		t.Fatalf("URL = %q, want presigned URL", out.URL)
	}
	cm, _ := newTestCore(ctx, t, notFoundStub, storeWith(t, split, nil), nil)
	if _, err := cm.PresignUpload(storage.WithSubject(ctx, "bob"), "bkt", "k", "text/plain", time.Minute); !errors.Is(
		err,
		storage.ErrForbidden,
	) {
		t.Fatalf("err = %v, want forbidden (bob lacks write)", err)
	}
}

func TestPresignUploadPresignError(t *testing.T) {
	c, _ := newTestCore(t.Context(), t, okStub, storage.PolicyStore{}, nil)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.PresignUpload(canceled, "bkt", "k", "text/plain", time.Minute); err == nil {
		t.Fatal("PresignUpload(canceled) = nil, want error")
	}
}

func TestPresignDownloadValidation(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	if _, err := c.PresignDownload(ctx, "bad bucket!", "k", time.Minute); err == nil {
		t.Fatal("PresignDownload(bad bucket) = nil, want error")
	}
	if _, err := c.PresignDownload(ctx, "bkt", "", time.Minute); err == nil {
		t.Fatal("PresignDownload(bad key) = nil, want error")
	}
	if _, err := c.PresignDownload(ctx, "bkt", "k", 0); err == nil {
		t.Fatal("PresignDownload(zero ttl) = nil, want error")
	}
	if _, err := c.PresignDownload(ctx, "bkt", "k", 8*24*time.Hour); err == nil {
		t.Fatal("PresignDownload(huge ttl) = nil, want error")
	}
}

func TestPresignDownloadUnconfigured(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	out := mustPresignDownload(ctx, t, c, "bkt", "dir/k")
	if out.Method != http.MethodGet {
		t.Fatalf("Method = %q, want GET", out.Method)
	}
	for _, want := range []string{"bkt", "dir/k", "X-Amz-Signature"} {
		if !strings.Contains(out.URL, want) {
			t.Fatalf("URL %q missing %q", out.URL, want)
		}
	}
}

func TestPresignDownloadPublic(t *testing.T) {
	ctx := t.Context()
	pol := &storage.Policy{Read: storage.Rule{Public: true}}
	c, stub := newTestCore(ctx, t, boomStub, storeWith(t, pol, nil), nil)
	out := mustPresignDownload(ctx, t, c, "bkt", "k")
	if out.Method != http.MethodGet {
		t.Fatalf("Method = %q, want GET", out.Method)
	}
	if !strings.Contains(out.URL, "bkt.s3.us-east-1.amazonaws.com") {
		t.Fatalf("URL = %q, want static amazonaws URL", out.URL)
	}
	if stub.calls != 0 {
		t.Fatalf("calls = %d, want 0 (static URL needs no HTTP)", stub.calls)
	}
}

func TestPresignDownloadStaticError(t *testing.T) {
	ctx := t.Context()
	pol := &storage.Policy{Read: storage.Rule{Public: true}}
	failing := func(_, _ string) (string, error) { return "", errBoom }
	c, _ := newTestCore(ctx, t, okStub, storeWith(t, pol, nil), failing)
	if _, err := c.PresignDownload(ctx, "bkt", "k", time.Minute); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestPresignDownloadPrivate(t *testing.T) {
	ctx := t.Context()
	pol := &storage.Policy{Read: storage.Rule{Allow: []storage.Subject{"alice"}}}
	c, _ := newTestCore(ctx, t, okStub, storeWith(t, pol, nil), nil)
	if _, err := c.PresignDownload(ctx, "bkt", "k", time.Minute); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	out := mustPresignDownload(storage.WithSubject(ctx, "alice"), t, c, "bkt", "k")
	if out.Method != http.MethodGet {
		t.Fatalf("Method = %q, want GET", out.Method)
	}
	if !strings.Contains(out.URL, "X-Amz-Signature") {
		t.Fatalf("URL = %q, want presigned URL", out.URL)
	}
}

func TestPresignDownloadPresignError(t *testing.T) {
	c, _ := newTestCore(t.Context(), t, okStub, storage.PolicyStore{}, nil)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.PresignDownload(canceled, "bkt", "k", time.Minute); err == nil {
		t.Fatal("PresignDownload(canceled) = nil, want error")
	}
}

func TestDelete(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, deleteOKStub, storage.PolicyStore{}, nil)
	if err := c.Delete(ctx, "bad bucket!", "k"); err == nil {
		t.Fatal("Delete(bad bucket) = nil, want error")
	}
	if err := c.Delete(ctx, "bkt", "k"); err != nil {
		t.Fatal(err)
	}
	cb, _ := newTestCore(ctx, t, boomStub, storage.PolicyStore{}, nil)
	if err := cb.Delete(ctx, "bkt", "k"); err == nil || !strings.Contains(err.Error(), "delete object") {
		t.Fatalf("err = %v, want delete object error", err)
	}
	denied := storeWith(t, &storage.Policy{}, nil)
	cd, _ := newTestCore(ctx, t, deleteOKStub, denied, nil)
	if err := cd.Delete(ctx, "bkt", "k"); !errors.Is(err, storage.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	allowed := storeWith(t, &storage.Policy{Delete: storage.Rule{Allow: []storage.Subject{"alice"}}}, nil)
	ca, _ := newTestCore(ctx, t, deleteOKStub, allowed, nil)
	if err := ca.Delete(storage.WithSubject(ctx, "alice"), "bkt", "k"); err != nil {
		t.Fatal(err)
	}
}

func TestExists(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	if _, err := c.Exists(ctx, "bad bucket!", "k"); err == nil {
		t.Fatal("Exists(bad bucket) = nil, want error")
	}
	if found, err := c.Exists(ctx, "bkt", "k"); err != nil || !found {
		t.Fatalf("Exists = (%v, %v), want (true, nil)", found, err)
	}
}

func TestExistsUnrestricted(t *testing.T) {
	ctx := t.Context()
	ok, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	if found, err := ok.ExistsUnrestricted(ctx, "bkt", "k"); err != nil || !found {
		t.Fatalf("ExistsUnrestricted(200) = (%v, %v), want (true, nil)", found, err)
	}
	missing, _ := newTestCore(ctx, t, notFoundStub, storage.PolicyStore{}, nil)
	if found, err := missing.ExistsUnrestricted(ctx, "bkt", "k"); err != nil || found {
		t.Fatalf("ExistsUnrestricted(404) = (%v, %v), want (false, nil)", found, err)
	}
	denied, _ := newTestCore(ctx, t, forbiddenStub, storage.PolicyStore{}, nil)
	if _, err := denied.ExistsUnrestricted(ctx, "bkt", "k"); err == nil {
		t.Fatal("ExistsUnrestricted(403) = nil, want error")
	}
	broken, _ := newTestCore(ctx, t, boomStub, storage.PolicyStore{}, nil)
	if _, err := broken.ExistsUnrestricted(ctx, "bkt", "k"); err == nil ||
		!strings.Contains(err.Error(), "head object") {
		t.Fatalf("err = %v, want head object error", err)
	}
}

type stubFn func(req *http.Request) (int, string, error)

func moveRouter(head, put, del stubFn) stubFn {
	return func(req *http.Request) (int, string, error) {
		switch req.Method {
		case http.MethodHead:
			return head(req)
		case http.MethodDelete:
			return del(req)
		default:
			return put(req)
		}
	}
}

func headByKey(src, dst stubFn) stubFn {
	return func(req *http.Request) (int, string, error) {
		if strings.Contains(req.URL.Path, "dstkey") {
			return dst(req)
		}
		return src(req)
	}
}

func TestMoveValidation(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	if err := c.Move(ctx, "bad bucket!", "k", "dst", "k"); err == nil {
		t.Fatal("Move(bad src bucket) = nil, want error")
	}
	if err := c.Move(ctx, "src", "k", "dst", ""); err == nil {
		t.Fatal("Move(bad dst key) = nil, want error")
	}
}

func TestMoveSelfNoop(t *testing.T) {
	ctx := t.Context()
	c, stub := newTestCore(ctx, t, boomStub, storage.PolicyStore{}, nil)
	if err := c.Move(ctx, "bkt", "k", "bkt", "k"); err != nil {
		t.Fatal(err)
	}
	if stub.calls != 0 {
		t.Fatalf("calls = %d, want 0 (self-move makes no HTTP calls)", stub.calls)
	}
}

func TestMoveSourceHeadError(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, boomStub, storage.PolicyStore{}, nil)
	if err := c.Move(ctx, "src", "k", "dst", "k"); err == nil ||
		!strings.Contains(err.Error(), "head object") {
		t.Fatalf("err = %v, want head object error", err)
	}
}

func TestMoveSourceMissing(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, notFoundStub, storage.PolicyStore{}, nil)
	if err := c.Move(ctx, "src", "k", "dst", "k"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestMoveSourceReadDeny(t *testing.T) {
	ctx := t.Context()
	pol := &storage.Policy{
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"alice"}},
		Delete: storage.Rule{Allow: []storage.Subject{"alice"}},
	}
	c, _ := newTestCore(ctx, t, okStub, storeWith(t, pol, nil), nil)
	if err := c.Move(storage.WithSubject(ctx, "alice"), "src", "k", "dst", "k"); !errors.Is(
		err,
		storage.ErrForbidden,
	) {
		t.Fatalf("err = %v, want forbidden (read denied)", err)
	}
}

func TestMoveSourceDeleteDeny(t *testing.T) {
	ctx := t.Context()
	pol := &storage.Policy{
		Read:   storage.Rule{Allow: []storage.Subject{"alice"}},
		Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
		Update: storage.Rule{Allow: []storage.Subject{"alice"}},
	}
	c, _ := newTestCore(ctx, t, okStub, storeWith(t, pol, nil), nil)
	if err := c.Move(storage.WithSubject(ctx, "alice"), "src", "k", "dst", "k"); !errors.Is(
		err,
		storage.ErrForbidden,
	) {
		t.Fatalf("err = %v, want forbidden (delete denied)", err)
	}
}

func TestMoveDstHeadError(t *testing.T) {
	ctx := t.Context()
	buckets := map[storage.BucketName]storage.Policy{
		"srcbkt": fullAccess(),
		"dstbkt": {
			Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
			Update: storage.Rule{Allow: []storage.Subject{"alice"}, Deny: []storage.Subject{"bob"}},
		},
	}
	fn := moveRouter(headByKey(okStub, boomStub), copyOKStub, deleteOKStub)
	c, _ := newTestCore(ctx, t, fn, storeWith(t, nil, buckets), nil)
	if err := c.Move(storage.WithSubject(ctx, "alice"), "srcbkt", "srckey", "dstbkt", "dstkey"); err == nil ||
		!strings.Contains(err.Error(), "head object") {
		t.Fatalf("err = %v, want head object error", err)
	}
}

func TestMoveDstExistsUpdateDeny(t *testing.T) {
	ctx := t.Context()
	buckets := map[storage.BucketName]storage.Policy{
		"srcbkt": fullAccess(),
		"dstbkt": {Write: storage.Rule{Allow: []storage.Subject{"alice"}}},
	}
	fn := moveRouter(okStub, copyOKStub, deleteOKStub)
	c, _ := newTestCore(ctx, t, fn, storeWith(t, nil, buckets), nil)
	if err := c.Move(storage.WithSubject(ctx, "alice"), "srcbkt", "srckey", "dstbkt", "dstkey"); !errors.Is(
		err,
		storage.ErrForbidden,
	) {
		t.Fatalf("err = %v, want forbidden (update denied)", err)
	}
}

func TestMoveDstDeny(t *testing.T) {
	ctx := t.Context()
	buckets := map[storage.BucketName]storage.Policy{"srcbkt": fullAccess()}
	fn := moveRouter(okStub, copyOKStub, deleteOKStub)
	c, _ := newTestCore(ctx, t, fn, storeWith(t, nil, buckets), nil)
	if err := c.Move(storage.WithSubject(ctx, "alice"), "srcbkt", "srckey", "dstbkt", "dstkey"); !errors.Is(
		err,
		storage.ErrForbidden,
	) {
		t.Fatalf("err = %v, want forbidden (dst write denied)", err)
	}
}

func TestMoveCopyError(t *testing.T) {
	ctx := t.Context()
	fn := moveRouter(okStub, boomStub, deleteOKStub)
	c, _ := newTestCore(ctx, t, fn, storage.PolicyStore{}, nil)
	if err := c.Move(ctx, "src", "k", "dst", "k"); err == nil ||
		!strings.Contains(err.Error(), "copy object") {
		t.Fatalf("err = %v, want copy object error", err)
	}
}

func TestMoveDeleteError(t *testing.T) {
	ctx := t.Context()
	fn := moveRouter(okStub, copyOKStub, boomStub)
	c, _ := newTestCore(ctx, t, fn, storage.PolicyStore{}, nil)
	if err := c.Move(ctx, "src", "k", "dst", "k"); err == nil ||
		!strings.Contains(err.Error(), "move copied but delete source failed") {
		t.Fatalf("err = %v, want move/delete error", err)
	}
}

func TestMoveSuccessUnconfigured(t *testing.T) {
	ctx := t.Context()
	fn := moveRouter(okStub, copyOKStub, deleteOKStub)
	c, _ := newTestCore(ctx, t, fn, storage.PolicyStore{}, nil)
	if err := c.Move(ctx, "src", "k", "dst", "k"); err != nil {
		t.Fatal(err)
	}
}

func TestMoveSuccessConfigured(t *testing.T) {
	ctx := t.Context()
	buckets := map[storage.BucketName]storage.Policy{
		"srcbkt": fullAccess(),
		"dstbkt": fullAccess(),
	}
	fn := moveRouter(okStub, copyOKStub, deleteOKStub)
	c, _ := newTestCore(ctx, t, fn, storeWith(t, nil, buckets), nil)
	if err := c.Move(storage.WithSubject(ctx, "alice"), "srcbkt", "srckey", "dstbkt", "dstkey"); err != nil {
		t.Fatal(err)
	}
}

func TestMoveSuccessConfiguredExistCheckMiss(t *testing.T) {
	ctx := t.Context()
	buckets := map[storage.BucketName]storage.Policy{
		"srcbkt": fullAccess(),
		"dstbkt": {
			Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
			Update: storage.Rule{Allow: []storage.Subject{"alice"}, Deny: []storage.Subject{"bob"}},
		},
	}
	fn := moveRouter(headByKey(okStub, notFoundStub), copyOKStub, deleteOKStub)
	c, _ := newTestCore(ctx, t, fn, storeWith(t, nil, buckets), nil)
	if err := c.Move(storage.WithSubject(ctx, "alice"), "srcbkt", "srckey", "dstbkt", "dstkey"); err != nil {
		t.Fatal(err)
	}
}

func TestMoveSuccessConfiguredExistCheckHit(t *testing.T) {
	ctx := t.Context()
	buckets := map[storage.BucketName]storage.Policy{
		"srcbkt": fullAccess(),
		"dstbkt": {
			Write:  storage.Rule{Allow: []storage.Subject{"alice"}},
			Update: storage.Rule{Allow: []storage.Subject{"alice"}, Deny: []storage.Subject{"bob"}},
		},
	}
	fn := moveRouter(okStub, copyOKStub, deleteOKStub)
	c, _ := newTestCore(ctx, t, fn, storeWith(t, nil, buckets), nil)
	if err := c.Move(storage.WithSubject(ctx, "alice"), "srcbkt", "srckey", "dstbkt", "dstkey"); err != nil {
		t.Fatal(err)
	}
}

func TestStaticURL(t *testing.T) {
	ctx := t.Context()
	c, _ := newTestCore(ctx, t, okStub, storage.PolicyStore{}, nil)
	c.URLBase = "https://cdn.example//"
	got, err := c.StaticURL("bkt", "a b/c")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://cdn.example/bkt/a%20b/c" {
		t.Fatalf("StaticURL = %q, want trimmed base with escaped key", got)
	}
	c.URLBase = ""
	got, err = c.StaticURL("bkt", "a/b")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://bkt.s3.us-east-1.amazonaws.com/a/b" {
		t.Fatalf("StaticURL = %q, want amazonaws format", got)
	}
}

func TestEscapeKey(t *testing.T) {
	if got := EscapeKey("a/b/c"); got != "a/b/c" {
		t.Fatalf("EscapeKey = %q, want slashes preserved", got)
	}
	if got := EscapeKey("a b/c"); got != "a%20b/c" {
		t.Fatalf("EscapeKey = %q, want segment escaping", got)
	}
	segs := []string{"héllo", "世界"}
	want := url.PathEscape(segs[0]) + "/" + url.PathEscape(segs[1])
	if got := EscapeKey("héllo/世界"); got != want {
		t.Fatalf("EscapeKey = %q, want %q", got, want)
	}
	if got := url.PathEscape("a/b"); !strings.Contains(got, "%2F") {
		t.Skip("assumption broken: PathEscape keeps slashes")
	}
	if got := EscapeKey("a/b"); strings.Contains(got, "%2F") {
		t.Fatalf("EscapeKey = %q, want slashes unescaped", got)
	}
}

func TestValidatePolicyCoherence(t *testing.T) {
	pub := storage.Rule{Public: true}
	writeOnly := storage.Policy{Write: pub}
	if err := ValidatePolicyCoherence("p", nil, writeOnly); err == nil {
		t.Fatal("write-public/update-private default = nil, want error")
	}
	updateOnly := storage.Policy{Update: pub}
	if err := ValidatePolicyCoherence("p", nil, updateOnly); err == nil {
		t.Fatal("update-public/write-private default = nil, want error")
	}
	buckets := map[storage.BucketName]storage.Policy{"photos": writeOnly}
	if err := ValidatePolicyCoherence("p", buckets, storage.Policy{}); err == nil ||
		!strings.Contains(err.Error(), `"photos"`) {
		t.Fatalf("err = %v, want bucket name in message", err)
	}
	if err := ValidatePolicyCoherence("p", nil, storage.Policy{}); err != nil {
		t.Fatal(err)
	}
	both := storage.Policy{Write: pub, Update: pub}
	clean := map[storage.BucketName]storage.Policy{"a": both}
	if err := ValidatePolicyCoherence("p", clean, both); err != nil {
		t.Fatal(err)
	}
}
