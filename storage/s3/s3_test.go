package s3

import (
	"encoding/json/jsontext"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/storage"
	"github.com/zenta-dev/zever/storage/s3core"
)

// stubTransport is an aws.HTTPClient with scripted responses.
type stubTransport struct {
	calls int
	do    func(*http.Request) (*http.Response, error)
}

func (s *stubTransport) Do(r *http.Request) (*http.Response, error) {
	s.calls++

	return s.do(r)
}

func newStubClient(t *testing.T, tr *stubTransport) *s3sdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test-key", "test-secret", "")),
	)
	if err != nil {
		t.Fatal(err)
	}

	return s3sdk.NewFromConfig(cfg, func(o *s3sdk.Options) {
		o.HTTPClient = tr
		o.Retryer = aws.NopRetryer{}
	})
}

func newTestAdapter(t *testing.T, cfg *storage.PolicyConfig, syncFail string, tr *stubTransport) *s3Adapter {
	t.Helper()

	var store storage.PolicyStore
	if err := store.ResolveFromConfig(cfg); err != nil {
		t.Fatal(err)
	}

	return &s3Adapter{
		Core:       s3core.New("s3", "us-east-1", "", newStubClient(t, tr), nil, store, nil),
		policySync: "auto",
		syncFail:   syncFail,
	}
}

func newPolicyOnlyAdapter(t *testing.T, cfg *storage.PolicyConfig) *s3Adapter {
	t.Helper()

	var store storage.PolicyStore
	if err := store.ResolveFromConfig(cfg); err != nil {
		t.Fatal(err)
	}

	return &s3Adapter{
		Core:       s3core.New("s3", "us-east-1", "", nil, nil, store, nil),
		policySync: "auto",
		syncFail:   "warn",
	}
}

func privatePolicyConfig() *storage.PolicyConfig {
	return &storage.PolicyConfig{Default: &storage.Policy{}}
}

func publicReadPolicyConfig() *storage.PolicyConfig {
	return &storage.PolicyConfig{Default: &storage.Policy{Read: storage.Rule{Public: true}}}
}

func xmlResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/xml"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func policyXML(doc string) string {
	return doc
}

func emptyPolicyXML() string {
	return ""
}

func mustAdapter(t *testing.T, st storage.Storage) *s3Adapter {
	t.Helper()

	a, ok := st.(*s3Adapter)
	if !ok {
		t.Fatal("New() did not return *s3Adapter")
	}

	if a.Core == nil {
		t.Fatal("New() returned adapter with nil Core")
	}

	return a
}

func TestNewValidationErrors(t *testing.T) {
	badPolicy := &storage.PolicyConfig{Buckets: map[storage.BucketName]storage.Policy{"bad bucket!": {}}}
	tests := []struct {
		name string
		opts storage.Options
	}{
		{name: "bad url_base", opts: storage.Options{URLBase: "://bad"}},
		{name: "bad policy", opts: storage.Options{Policy: badPolicy}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opts); err == nil {
				t.Fatal("New() = nil, want error")
			}
		})
	}
}

func TestNewRegionDefault(t *testing.T) {
	a := mustAdapter(t, mustNew(t, storage.Options{}))
	if a.Region != "us-east-1" {
		t.Fatalf("Region = %q, want %q", a.Region, "us-east-1")
	}
	if a.policySync != "manual" {
		t.Fatalf("policySync = %q, want %q", a.policySync, "manual")
	}
	if a.syncFail != "warn" {
		t.Fatalf("syncFail = %q, want %q", a.syncFail, "warn")
	}
}

func mustNew(t *testing.T, opts storage.Options) storage.Storage {
	t.Helper()

	st, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}

	return st
}

func TestNewRegionCustom(t *testing.T) {
	a := mustAdapter(t, mustNew(t, storage.Options{Region: "eu-west-1"}))
	if a.Region != "eu-west-1" {
		t.Fatalf("Region = %q, want %q", a.Region, "eu-west-1")
	}
}

func TestNewSuccessExplicit(t *testing.T) {
	opts := storage.Options{
		Region:     "eu-west-1",
		Endpoint:   "https://s3.example.com",
		URLBase:    "https://cdn.example.com",
		PolicySync: "auto",
		SyncFail:   "require",
	}
	a := mustAdapter(t, mustNew(t, opts))
	if a.policySync != "auto" {
		t.Fatalf("policySync = %q, want %q", a.policySync, "auto")
	}
	if a.syncFail != "require" {
		t.Fatalf("syncFail = %q, want %q", a.syncFail, "require")
	}
}

func TestNewSuccessWithPolicy(t *testing.T) {
	a := mustAdapter(t, mustNew(t, storage.Options{Policy: publicReadPolicyConfig()}))
	if !a.Configured() {
		t.Fatal("Configured() = false, want true")
	}
}

func TestNewCoherenceError(t *testing.T) {
	opts := storage.Options{Policy: &storage.PolicyConfig{
		Default: &storage.Policy{Write: storage.Rule{Public: true}},
	}}
	_, err := New(opts)
	if err == nil || !strings.Contains(err.Error(), "public write") {
		t.Fatalf("New() err = %v, want public-write coherence error", err)
	}
}

func TestNewClientError(t *testing.T) {
	opts := storage.Options{Endpoint: "ftp://invalid.example.com", URLBase: "https://cdn.example.com"}
	_, err := New(opts)
	if err == nil {
		t.Fatal("New() = nil, want NewClient error")
	}
}

func TestAdapterCloseAndName(t *testing.T) {
	st := mustNew(t, storage.Options{})
	if err := st.Close(t.Context()); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if got := st.Name(); got != "s3" {
		t.Fatalf("Name() = %q, want %q", got, "s3")
	}
}

func TestSyncPolicyManual(t *testing.T) {
	a := &s3Adapter{policySync: "manual"}
	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}
}

func TestPolicySyncErrorBranches(t *testing.T) {
	sentinel := errors.New("sync boom")

	req := &s3Adapter{syncFail: "require"}
	if err := req.policySyncError(t.Context(), "b", sentinel); !errors.Is(err, sentinel) {
		t.Fatalf("policySyncError() = %v, want sentinel", err)
	}

	warn := &s3Adapter{
		Core:     s3core.New("s3", "us-east-1", "", nil, nil, storage.PolicyStore{}, nil),
		syncFail: "warn",
	}
	if err := warn.policySyncError(t.Context(), "b", sentinel); err != nil {
		t.Fatalf("policySyncError() = %v, want nil", err)
	}
}

func TestSyncPolicyGetErrorRequire(t *testing.T) {
	tr := &stubTransport{do: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("get boom")
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "require", tr)
	err := a.SyncPolicy(t.Context(), "b")
	if err == nil || !strings.Contains(err.Error(), "get bucket policy") {
		t.Fatalf("SyncPolicy() = %v, want get-bucket-policy error", err)
	}
}

func TestSyncPolicyGetErrorWarn(t *testing.T) {
	tr := &stubTransport{do: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("get boom")
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "warn", tr)
	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}
}

func TestSyncPolicyMergeError(t *testing.T) {
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, policyXML("{invalid-json")), nil
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "require", tr)
	err := a.SyncPolicy(t.Context(), "b")
	if err == nil || !strings.Contains(err.Error(), "parse existing") {
		t.Fatalf("SyncPolicy() = %v, want parse-existing error", err)
	}
}

func TestSyncPolicyMergeErrorWarn(t *testing.T) {
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, policyXML("{invalid-json")), nil
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "warn", tr)
	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}
}

func TestSyncPolicyNoDriftNoPut(t *testing.T) {
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 404, ""), nil
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "require", tr)
	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}
	if tr.calls != 1 {
		t.Fatalf("client calls = %d, want 1 (get only, no put)", tr.calls)
	}
}

func TestSyncPolicyDriftPuts(t *testing.T) {
	current := `{"Version":"2012-10-17","Statement":[{"Sid":"zever-read","Effect":"Deny",` +
		`"Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":["arn:aws:s3:::b/*"]}]}`

	var puts int
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPut {
			puts++

			return xmlResponse(r, 200, ""), nil
		}
		return xmlResponse(r, 200, policyXML(current)), nil
	}}
	cfg := &storage.PolicyConfig{Buckets: map[storage.BucketName]storage.Policy{
		"b": {Read: storage.Rule{Public: true}},
	}}
	a := newTestAdapter(t, cfg, "require", tr)
	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}
	if puts != 1 {
		t.Fatalf("puts = %d, want 1", puts)
	}
}

func TestSyncPolicyPutError(t *testing.T) {
	newPutFailStub := func() *stubTransport {
		return &stubTransport{do: func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodPut {
				return nil, errors.New("put boom")
			}
			return xmlResponse(r, 404, ""), nil
		}}
	}

	a := newTestAdapter(t, publicReadPolicyConfig(), "require", newPutFailStub())
	err := a.SyncPolicy(t.Context(), "b")
	if err == nil || !strings.Contains(err.Error(), "put bucket policy") {
		t.Fatalf("SyncPolicy() = %v, want put-bucket-policy error", err)
	}

	a = newTestAdapter(t, publicReadPolicyConfig(), "warn", newPutFailStub())
	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}
}

func TestSyncPolicyPutOK(t *testing.T) {
	var puts int
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPut {
			puts++

			return xmlResponse(r, 200, ""), nil
		}
		return xmlResponse(r, 404, ""), nil
	}}
	a := newTestAdapter(t, publicReadPolicyConfig(), "require", tr)
	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}
	if puts != 1 {
		t.Fatalf("puts = %d, want 1", puts)
	}
}

func TestGetBucketPolicyValue(t *testing.T) {
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, policyXML("hello-doc")), nil
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "warn", tr)
	got, err := a.getBucketPolicy(t.Context(), "b")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello-doc" {
		t.Fatalf("getBucketPolicy() = %q, want %q", got, "hello-doc")
	}
}

func TestGetBucketPolicyNilPolicy(t *testing.T) {
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, emptyPolicyXML()), nil
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "warn", tr)
	got, err := a.getBucketPolicy(t.Context(), "b")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("getBucketPolicy() = %q, want empty", got)
	}
}

func TestGetBucketPolicyNotFound(t *testing.T) {
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 404, ""), nil
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "warn", tr)
	got, err := a.getBucketPolicy(t.Context(), "b")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("getBucketPolicy() = %q, want empty", got)
	}
}

func TestGetBucketPolicyError(t *testing.T) {
	tr := &stubTransport{do: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport boom")
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "warn", tr)
	if _, err := a.getBucketPolicy(t.Context(), "b"); err == nil {
		t.Fatal("getBucketPolicy() = nil, want error")
	}
}

func TestPutBucketPolicyError(t *testing.T) {
	tr := &stubTransport{do: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("put boom")
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "warn", tr)
	if err := a.putBucketPolicy(t.Context(), "b", "{}"); err == nil {
		t.Fatal("putBucketPolicy() = nil, want error")
	}
}

func TestPutBucketPolicyOK(t *testing.T) {
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, ""), nil
	}}
	a := newTestAdapter(t, privatePolicyConfig(), "warn", tr)
	if err := a.putBucketPolicy(t.Context(), "b", "{}"); err != nil {
		t.Fatalf("putBucketPolicy() = %v, want nil", err)
	}
}

func TestBucketPolicyDocPrivate(t *testing.T) {
	doc := bucketPolicyDoc(storage.Policy{}, "b")
	if strings.Contains(doc, "zever-") {
		t.Fatalf("bucketPolicyDoc() = %s, want no statements", doc)
	}
}

func TestBucketPolicyDocPublic(t *testing.T) {
	p := storage.Policy{
		Read:   storage.Rule{Public: true},
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
		Delete: storage.Rule{Public: true},
	}
	doc := bucketPolicyDoc(p, "mybucket")
	if got := strings.Count(doc, `"Sid"`); got != 4 {
		t.Fatalf("statements = %d, want 4", got)
	}
	for _, sid := range []string{"zever-read", "zever-write", "zever-update", "zever-delete"} {
		if !strings.Contains(doc, sid) {
			t.Fatalf("bucketPolicyDoc() missing %q in %s", sid, doc)
		}
	}
	for _, action := range []string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject"} {
		if !strings.Contains(doc, action) {
			t.Fatalf("bucketPolicyDoc() missing %q in %s", action, doc)
		}
	}
	if !strings.Contains(doc, "arn:aws:s3:::mybucket/*") {
		t.Fatalf("bucketPolicyDoc() missing resource arn in %s", doc)
	}
}

func TestBucketPolicyDocSinglePerm(t *testing.T) {
	p := storage.Policy{Delete: storage.Rule{Public: true}}
	doc := bucketPolicyDoc(p, "b")
	if got := strings.Count(doc, `"Sid"`); got != 1 {
		t.Fatalf("statements = %d, want 1", got)
	}
	if !strings.Contains(doc, "s3:DeleteObject") {
		t.Fatalf("bucketPolicyDoc() missing delete action in %s", doc)
	}
}

func TestMergePolicyEmpty(t *testing.T) {
	a := newPolicyOnlyAdapter(t, privatePolicyConfig())
	merged, drift, err := a.mergePolicy("b", "")
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatal("drift = true, want false")
	}
	if !strings.Contains(merged, `"Version": "2012-10-17"`) {
		t.Fatalf("merged missing version in %s", merged)
	}
	if !strings.Contains(merged, `"Statement": []`) {
		t.Fatalf("merged missing empty statements in %s", merged)
	}
}

func TestMergePolicyVersionFill(t *testing.T) {
	a := newPolicyOnlyAdapter(t, privatePolicyConfig())
	merged, drift, err := a.mergePolicy("b", `{"Statement":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatal("drift = true, want false")
	}
	if !strings.Contains(merged, `"Version": "2012-10-17"`) {
		t.Fatalf("merged missing filled version in %s", merged)
	}
}

func TestMergePolicyExplicitEmptyVersion(t *testing.T) {
	a := newPolicyOnlyAdapter(t, privatePolicyConfig())
	merged, drift, err := a.mergePolicy("b", `{"Version":"","Statement":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatal("drift = true, want false")
	}
	if !strings.Contains(merged, `"Version": "2012-10-17"`) {
		t.Fatalf("merged missing filled version in %s", merged)
	}
}

func TestMergePolicyExplicitNullStatements(t *testing.T) {
	a := newPolicyOnlyAdapter(t, privatePolicyConfig())
	merged, drift, err := a.mergePolicy("b", `{"Version":"2012-10-17","Statement":null}`)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatal("drift = true, want false")
	}
	if !strings.Contains(merged, `"Statement": []`) {
		t.Fatalf("merged missing empty statements in %s", merged)
	}
}

func TestMergePolicyNilStatements(t *testing.T) {
	a := newPolicyOnlyAdapter(t, privatePolicyConfig())
	merged, drift, err := a.mergePolicy("b", `{"Version":"2012-10-17"}`)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatal("drift = true, want false")
	}
	if !strings.Contains(merged, `"Statement": []`) {
		t.Fatalf("merged missing empty statements in %s", merged)
	}
}

func TestMergePolicyKeepsForeign(t *testing.T) {
	a := newPolicyOnlyAdapter(t, privatePolicyConfig())
	current := `{"Version":"2012-10-17","Statement":[{"Sid":"admin","Effect":"Allow"}]}`
	merged, drift, err := a.mergePolicy("b", current)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatal("drift = true, want false")
	}
	if !strings.Contains(merged, `"admin"`) {
		t.Fatalf("merged dropped foreign statement in %s", merged)
	}
}

func TestMergePolicyKeepsUnparsableSid(t *testing.T) {
	a := newPolicyOnlyAdapter(t, privatePolicyConfig())
	current := `{"Version":"2012-10-17","Statement":["justastring",42]}`
	merged, drift, err := a.mergePolicy("b", current)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatal("drift = true, want false")
	}
	if !strings.Contains(merged, `"justastring"`) {
		t.Fatalf("merged dropped unparsable statement in %s", merged)
	}
}

func TestMergePolicyOursEqual(t *testing.T) {
	a := newPolicyOnlyAdapter(t, publicReadPolicyConfig())
	gen := bucketPolicyDoc(*publicReadPolicyConfig().Default, "b")
	_, drift, err := a.mergePolicy("b", gen)
	if err != nil {
		t.Fatal(err)
	}
	if drift {
		t.Fatal("drift = true, want false")
	}
}

func TestMergePolicyOursDrift(t *testing.T) {
	a := newPolicyOnlyAdapter(t, publicReadPolicyConfig())
	current := `{"Version":"2012-10-17","Statement":[{"Sid":"zever-read","Effect":"Deny",` +
		`"Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":["arn:aws:s3:::b/*"]}]}`
	merged, drift, err := a.mergePolicy("b", current)
	if err != nil {
		t.Fatal(err)
	}
	if !drift {
		t.Fatal("drift = false, want true")
	}
	if !strings.Contains(merged, `"Allow"`) {
		t.Fatalf("merged missing regenerated statement in %s", merged)
	}
}

func TestMergePolicyBadExisting(t *testing.T) {
	a := newPolicyOnlyAdapter(t, privatePolicyConfig())
	if _, _, err := a.mergePolicy("b", "{invalid"); err == nil {
		t.Fatal("mergePolicy() = nil, want error")
	}
}

func TestCanonicalPolicy(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantContains []string
		wantErr      bool
	}{
		{name: "empty", input: "", wantContains: []string{`"Version": "2012-10-17"`, `"Statement": []`}},
		{name: "whitespace", input: "  \n ", wantContains: []string{`"Version": "2012-10-17"`}},
		{name: "invalid", input: "{oops", wantErr: true},
		{name: "version fill", input: `{"Statement":[]}`, wantContains: []string{`"Version": "2012-10-17"`}},
		{name: "statements fill", input: `{"Version":"2012-10-17"}`, wantContains: []string{`"Statement": []`}},
		{
			name:         "roundtrip",
			input:        `{"Version":"2012-10-17","Statement":[{"Sid":"x","Effect":"Allow"}]}`,
			wantContains: []string{`"x"`, `"Allow"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := canonicalPolicy(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("canonicalPolicy() = nil, want error")
				}

				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Fatalf("canonicalPolicy() missing %q in %s", want, got)
				}
			}
		})
	}
}

func TestMarshalRawPolicy(t *testing.T) {
	got, err := marshalRawPolicy(rawPolicyDocument{Version: policyVersion})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, policyVersion) {
		t.Fatalf("marshalRawPolicy() missing version in %s", got)
	}
}

func TestMarshalRawPolicyError(t *testing.T) {
	doc := rawPolicyDocument{Version: policyVersion, Statement: []jsontext.Value{jsontext.Value("{invalid")}}
	if _, err := marshalRawPolicy(doc); err == nil {
		t.Fatal("marshalRawPolicy() = nil, want error")
	}
}

func TestCanonicalStatementsSorted(t *testing.T) {
	a := jsontext.Value(`{"Sid":"b","Effect":"Allow"}`)
	b := jsontext.Value(`{"Effect":"Allow","Sid":"a"}`)

	ab := canonicalStatements([]jsontext.Value{a, b})
	ba := canonicalStatements([]jsontext.Value{b, a})
	if ab != ba {
		t.Fatalf("order-dependent: %q vs %q", ab, ba)
	}

	reordered := jsontext.Value(`{"Effect":"Allow","Sid":"b"}`)
	norm := canonicalStatements([]jsontext.Value{reordered})
	single := canonicalStatements([]jsontext.Value{a})
	if norm != single {
		t.Fatalf("key order not normalized: %q vs %q", norm, single)
	}
}
