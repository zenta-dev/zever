package r2

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

// TestEdgeNewAccountIDBoundaries covers account-ID length and character edges.
func TestEdgeNewAccountIDBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID string
		want      bool
	}{
		{name: "single char", accountID: "a", want: true},
		{name: "digits", accountID: "0123456789", want: true},
		{name: "long", accountID: strings.Repeat("a", 256), want: true},
		{name: "space", accountID: "a b", want: false},
		{name: "dot", accountID: "a.b", want: false},
		{name: "colon", accountID: "a:b", want: false},
		{name: "underscore dash", accountID: "A-b_C", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isValidAccountID(tt.accountID); got != tt.want {
				t.Errorf("isValidAccountID(%q) = %v, want %v", tt.accountID, got, tt.want)
			}
		})
	}
}

// TestEdgeR2EndpointPassThrough returns a custom endpoint unchanged.
func TestEdgeR2EndpointPassThrough(t *testing.T) {
	t.Parallel()

	got, err := r2Endpoint("acct", "https://custom.example.com/")
	if err != nil {
		t.Fatal(err)
	}

	if got != "https://custom.example.com/" {
		t.Errorf("r2Endpoint = %q, want passthrough", got)
	}
}

// TestEdgeNewHTTPEndpoint accepts an http custom endpoint.
func TestEdgeNewHTTPEndpoint(t *testing.T) {
	t.Parallel()

	opts := validR2Options()
	opts.Endpoint = "http://custom.example.com"

	if _, err := New(opts); err != nil {
		t.Fatalf("New() = %v, want nil", err)
	}
}

// TestEdgeNewPublicWriteOnlyWithPubBase accepts a write-only public policy
// when a public base is configured.
func TestEdgeNewPublicWriteOnlyWithPubBase(t *testing.T) {
	t.Parallel()

	opts := validR2Options()
	opts.PublicURL = "https://pub.example.com"
	opts.Policy = &storage.PolicyConfig{Default: &storage.Policy{
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}}

	if _, err := New(opts); err != nil {
		t.Fatalf("New() = %v, want nil", err)
	}
}

// TestEdgePresignUploadValidation checks bucket/key/ttl validation on the
// unconfigured path (offline presigning).
func TestEdgePresignUploadValidation(t *testing.T) {
	t.Parallel()

	a, ok := mustNew(t, validR2Options()).(*r2Adapter)
	if !ok {
		t.Fatal("New() did not return *r2Adapter")
	}

	ctx := t.Context()

	if _, err := a.PresignUpload(ctx, "", "k", "", time.Hour); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Errorf("empty bucket = %v, want ErrInvalidBucket", err)
	}

	if _, err := a.PresignUpload(ctx, "bkt", "../x", "", time.Hour); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("bad key = %v, want ErrInvalidKey", err)
	}

	if _, err := a.PresignUpload(ctx, "bkt", "k", "", 0); err == nil {
		t.Error("zero ttl = nil, want error")
	}

	if _, err := a.PresignUpload(ctx, "bkt", "k", "", storage.MaxPresignTTL); err != nil {
		t.Errorf("max ttl = %v, want nil", err)
	}

	if _, err := a.PresignUpload(ctx, "bkt", "k", "", storage.MaxPresignTTL+time.Nanosecond); err == nil {
		t.Error("max ttl + 1ns = nil, want error")
	}
}

// TestEdgePresignDownloadValidation checks bucket/key/ttl validation on the
// unconfigured path.
func TestEdgePresignDownloadValidation(t *testing.T) {
	t.Parallel()

	a, ok := mustNew(t, validR2Options()).(*r2Adapter)
	if !ok {
		t.Fatal("New() did not return *r2Adapter")
	}

	ctx := t.Context()

	if _, err := a.PresignDownload(ctx, "", "k", time.Hour); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Errorf("empty bucket = %v, want ErrInvalidBucket", err)
	}

	if _, err := a.PresignDownload(ctx, "bkt", "../x", time.Hour); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("bad key = %v, want ErrInvalidKey", err)
	}

	if _, err := a.PresignDownload(ctx, "bkt", "k", -time.Second); err == nil {
		t.Error("negative ttl = nil, want error")
	}

	if _, err := a.PresignDownload(ctx, "bkt", "k", storage.MaxPresignTTL); err != nil {
		t.Errorf("max ttl = %v, want nil", err)
	}

	if _, err := a.PresignDownload(ctx, "bkt", "k", storage.MaxPresignTTL+time.Nanosecond); err == nil {
		t.Error("max ttl + 1ns = nil, want error")
	}
}

// TestEdgeStaticURLDefaultShape reports the amazonaws URL when no URL base is
// configured.
func TestEdgeStaticURLDefaultShape(t *testing.T) {
	t.Parallel()

	a, ok := mustNew(t, validR2Options()).(*r2Adapter)
	if !ok {
		t.Fatal("New() did not return *r2Adapter")
	}

	got, err := a.StaticURL("bkt", "dir/k")
	if err != nil {
		t.Fatal(err)
	}

	if want := "https://bkt.s3.auto.amazonaws.com/dir/k"; got != want {
		t.Errorf("StaticURL = %q, want %q", got, want)
	}
}

// TestEdgeStaticURLEscaping checks segment-wise key escaping against a
// configured URL base.
func TestEdgeStaticURLEscaping(t *testing.T) {
	t.Parallel()

	opts := validR2Options()
	opts.URLBase = "https://cdn.example.com/base/"

	a, ok := mustNew(t, opts).(*r2Adapter)
	if !ok {
		t.Fatal("New() did not return *r2Adapter")
	}

	got, err := a.StaticURL("my bucket", "a b/c+d")
	if err != nil {
		t.Fatal(err)
	}

	if want := "https://cdn.example.com/base/my%20bucket/a%20b/c+d"; got != want {
		t.Errorf("StaticURL = %q, want %q", got, want)
	}
}

// TestEdgePresignUploadStaticEscaping checks key escaping in public static
// upload URLs.
func TestEdgePresignUploadStaticEscaping(t *testing.T) {
	t.Parallel()

	opts := validR2Options()
	opts.PublicURL = "https://pub.example.com"
	opts.Policy = &storage.PolicyConfig{Default: &storage.Policy{
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
	}}

	a, ok := mustNew(t, opts).(*r2Adapter)
	if !ok {
		t.Fatal("New() did not return *r2Adapter")
	}

	got, err := a.PresignUpload(t.Context(), "bkt", "a b/c", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if want := "https://pub.example.com/bkt/a%20b/c"; got.URL != want {
		t.Errorf("PresignUpload URL = %q, want %q", got.URL, want)
	}
}

// TestEdgeCheckR2PublicPolicyEmpty covers nil policy maps with and without a
// public base.
func TestEdgeCheckR2PublicPolicyEmpty(t *testing.T) {
	t.Parallel()

	if err := checkR2PublicPolicy(nil, storage.Policy{}, ""); err != nil {
		t.Errorf("nil policies = %v, want nil", err)
	}

	if err := checkR2PublicPolicy(nil, storage.Policy{}, "https://pub.example.com"); err != nil {
		t.Errorf("nil policies with pubBase = %v, want nil", err)
	}
}
