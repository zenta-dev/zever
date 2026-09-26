package r2

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/storage"
)

func validR2Options() storage.Options {
	return storage.Options{
		AccountID:       "testaccount123",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
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

func TestNewValidationErrors(t *testing.T) {
	badPolicy := &storage.PolicyConfig{Buckets: map[storage.BucketName]storage.Policy{"bad bucket!": {}}}
	tests := []struct {
		name string
		opts storage.Options
	}{
		{name: "bad url_base", opts: storage.Options{URLBase: "://bad"}},
		{name: "bad policy", opts: func() storage.Options {
			o := validR2Options()
			o.Policy = badPolicy

			return o
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opts); err == nil {
				t.Fatal("New() = nil, want error")
			}
		})
	}
}

func TestNewAccountIDRequired(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
	}{
		{name: "empty", accountID: ""},
		{name: "whitespace", accountID: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := validR2Options()
			opts.AccountID = tt.accountID
			_, err := New(opts)
			if err == nil || !strings.Contains(err.Error(), `"account_id"`) {
				t.Fatalf("New() = %v, want account_id error", err)
			}
		})
	}
}

func TestNewCredentialsRequired(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*storage.Options)
		want   string
	}{
		{name: "missing key", mutate: func(o *storage.Options) { o.AccessKeyID = "" }, want: `"access_key_id"`},
		{name: "missing secret", mutate: func(o *storage.Options) { o.SecretAccessKey = " " }, want: `"secret_access_key"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := validR2Options()
			tt.mutate(&opts)
			_, err := New(opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() = %v, want %s error", err, tt.want)
			}
		})
	}
}

func TestNewRegionDefault(t *testing.T) {
	st := mustNew(t, validR2Options())
	a, ok := st.(*r2Adapter)
	if !ok {
		t.Fatal("New() did not return *r2Adapter")
	}
	if a.Region != "auto" {
		t.Fatalf("Region = %q, want %q", a.Region, "auto")
	}
}

func TestNewRegionCustom(t *testing.T) {
	opts := validR2Options()
	opts.Region = "weur"
	a, ok := mustNew(t, opts).(*r2Adapter)
	if !ok {
		t.Fatal("New() did not return *r2Adapter")
	}
	if a.Region != "weur" {
		t.Fatalf("Region = %q, want %q", a.Region, "weur")
	}
}

func TestNewInvalidAccountID(t *testing.T) {
	opts := validR2Options()
	opts.AccountID = "bad id!"
	_, err := New(opts)
	if err == nil || !strings.Contains(err.Error(), `"account_id"`) {
		t.Fatalf("New() = %v, want account_id error", err)
	}
}

func TestNewCustomEndpoint(t *testing.T) {
	opts := validR2Options()
	opts.Endpoint = "https://custom.example.com"
	st := mustNew(t, opts)
	if st == nil {
		t.Fatal("New() = nil, want adapter")
	}
}

func TestNewDefaultEndpoint(t *testing.T) {
	st := mustNew(t, validR2Options())
	if st == nil {
		t.Fatal("New() = nil, want adapter")
	}
}

func TestNewPublicURLInvalid(t *testing.T) {
	opts := validR2Options()
	opts.PublicURL = "bogus"
	_, err := New(opts)
	if err == nil {
		t.Fatal("New() = nil, want public_url error")
	}
}

func TestNewBucketPublicPolicyError(t *testing.T) {
	opts := validR2Options()
	opts.Policy = &storage.PolicyConfig{Buckets: map[storage.BucketName]storage.Policy{
		"b": {Read: storage.Rule{Public: true}},
	}}
	_, err := New(opts)
	if err == nil || !strings.Contains(err.Error(), "public_url_base") {
		t.Fatalf("New() = %v, want public_url_base error", err)
	}
}

func TestNewDefaultPublicPolicyError(t *testing.T) {
	opts := validR2Options()
	opts.Policy = &storage.PolicyConfig{Default: &storage.Policy{Read: storage.Rule{Public: true}}}
	_, err := New(opts)
	if err == nil || !strings.Contains(err.Error(), "public_url_base") {
		t.Fatalf("New() = %v, want public_url_base error", err)
	}
}

func TestNewCoherenceError(t *testing.T) {
	opts := validR2Options()
	opts.PublicURL = "https://pub.example.com"
	opts.Policy = &storage.PolicyConfig{Default: &storage.Policy{Write: storage.Rule{Public: true}}}
	_, err := New(opts)
	if err == nil || !strings.Contains(err.Error(), "public write") {
		t.Fatalf("New() = %v, want public-write coherence error", err)
	}
}

func TestNewClientError(t *testing.T) {
	opts := validR2Options()
	opts.Endpoint = "ftp://bad.example.com"
	_, err := New(opts)
	if err == nil {
		t.Fatal("New() = nil, want NewClient error")
	}
}

func TestPresignDownloadStatic(t *testing.T) {
	opts := validR2Options()
	opts.PublicURL = "https://pub.example.com/"
	opts.Policy = &storage.PolicyConfig{Default: &storage.Policy{Read: storage.Rule{Public: true}}}
	st := mustNew(t, opts)

	got, err := st.PresignDownload(t.Context(), "mybucket", "dir/file.txt", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodGet {
		t.Fatalf("Method = %q, want %q", got.Method, http.MethodGet)
	}
	want := "https://pub.example.com/mybucket/dir/file.txt"
	if got.URL != want {
		t.Fatalf("URL = %q, want %q", got.URL, want)
	}
}

func TestPresignUploadStatic(t *testing.T) {
	opts := validR2Options()
	opts.PublicURL = "https://pub.example.com"
	opts.Policy = &storage.PolicyConfig{Default: &storage.Policy{
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}}
	st := mustNew(t, opts)

	got, err := st.PresignUpload(t.Context(), "mybucket", "k", "text/plain", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPut {
		t.Fatalf("Method = %q, want %q", got.Method, http.MethodPut)
	}
	want := "https://pub.example.com/mybucket/k"
	if got.URL != want {
		t.Fatalf("URL = %q, want %q", got.URL, want)
	}
}

func TestAdapterNameAndClose(t *testing.T) {
	st := mustNew(t, validR2Options())
	if got := st.Name(); got != "r2" {
		t.Fatalf("Name() = %q, want %q", got, "r2")
	}
	if err := st.Close(t.Context()); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
}

func TestIsValidAccountID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "", want: false},
		{input: "abc123", want: true},
		{input: "a-b_cZ09", want: true},
		{input: "bad id!", want: false},
		{input: "a/b", want: false},
		{input: "caf\xc3\xa9", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isValidAccountID(tt.input); got != tt.want {
				t.Fatalf("isValidAccountID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestR2Endpoint(t *testing.T) {
	if _, err := r2Endpoint("bad id!", ""); err == nil {
		t.Fatal("r2Endpoint() = nil, want error")
	}

	got, err := r2Endpoint("testaccount123", "https://custom.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://custom.example.com" {
		t.Fatalf("r2Endpoint() = %q, want custom endpoint", got)
	}

	got, err = r2Endpoint("testaccount123", "")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://testaccount123.r2.cloudflarestorage.com"
	if got != want {
		t.Fatalf("r2Endpoint() = %q, want %q", got, want)
	}
}

func TestHasPublicPerm(t *testing.T) {
	if hasPublicPerm(storage.Policy{}) {
		t.Fatal("hasPublicPerm(private) = true, want false")
	}
	tests := []struct {
		name   string
		policy storage.Policy
	}{
		{name: "read", policy: storage.Policy{Read: storage.Rule{Public: true}}},
		{name: "write", policy: storage.Policy{Write: storage.Rule{Public: true}}},
		{name: "update", policy: storage.Policy{Update: storage.Rule{Public: true}}},
		{name: "delete", policy: storage.Policy{Delete: storage.Rule{Public: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !hasPublicPerm(tt.policy) {
				t.Fatal("hasPublicPerm() = false, want true")
			}
		})
	}
}

func TestCheckR2PublicPolicy(t *testing.T) {
	public := storage.Policy{Read: storage.Rule{Public: true}}

	if err := checkR2PublicPolicy(
		map[storage.BucketName]storage.Policy{"b": public},
		storage.Policy{},
		"https://pub.example.com",
	); err != nil {
		t.Fatalf("checkR2PublicPolicy() = %v, want nil", err)
	}

	if err := checkR2PublicPolicy(
		map[storage.BucketName]storage.Policy{"b": public},
		storage.Policy{},
		"",
	); err == nil || !strings.Contains(err.Error(), `"b"`) {
		t.Fatalf("checkR2PublicPolicy() = %v, want bucket error", err)
	}

	if err := checkR2PublicPolicy(nil, public, ""); err == nil ||
		!strings.Contains(err.Error(), "default policy") {
		t.Fatalf("checkR2PublicPolicy() = %v, want default-policy error", err)
	}

	if err := checkR2PublicPolicy(nil, storage.Policy{}, ""); err != nil {
		t.Fatalf("checkR2PublicPolicy() = %v, want nil", err)
	}
}
