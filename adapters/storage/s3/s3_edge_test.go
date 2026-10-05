package s3

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/shared/s3opts"
)

// TestEdgeNewEndpointWithoutURLBase rejects a custom endpoint when url_base is
// unset.
func TestEdgeNewEndpointWithoutURLBase(t *testing.T) {
	t.Parallel()

	_, err := New(storage.Options{Endpoint: "https://s3.example.com"})
	if err == nil || !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("New() = %v, want ErrInvalidOption", err)
	}
}

// TestEdgeNewPolicySyncInvalid rejects unknown policy_sync values.
func TestEdgeNewPolicySyncInvalid(t *testing.T) {
	t.Parallel()

	_, err := New(storage.Options{PolicySync: "sometimes"})
	if err == nil || !errors.Is(err, ErrInvalidOption) || !strings.Contains(err.Error(), "policy_sync") {
		t.Fatalf("New() = %v, want policy_sync error", err)
	}
}

// TestEdgeNewSyncFailInvalid rejects unknown sync_fail values.
func TestEdgeNewSyncFailInvalid(t *testing.T) {
	t.Parallel()

	_, err := New(storage.Options{SyncFail: "ignore"})
	if err == nil || !errors.Is(err, ErrInvalidOption) || !strings.Contains(err.Error(), "sync_fail") {
		t.Fatalf("New() = %v, want sync_fail error", err)
	}
}

// TestEdgeSyncPolicyNilPolicyField treats a 200 response with a nil Policy
// field as no policy.
func TestEdgeSyncPolicyNilPolicyField(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, ""), nil
	}}
	a := stubAdapter(t, privatePolicyConfig(), "require", tr)

	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}

	if tr.calls != 1 {
		t.Fatalf("client calls = %d, want 1 (get only, no put)", tr.calls)
	}
}

// TestEdgeSyncPolicyCanonicalNoDrift treats a semantically equal policy with
// reordered keys as in-sync: no drift decision fires.
func TestEdgeSyncPolicyCanonicalNoDrift(t *testing.T) {
	t.Parallel()

	current := `{"Statement":[{"Resource":["arn:aws:s3:::b/*"],"Action":"s3:GetObject",` +
		`"Principal":{"AWS":"*"},"Effect":"Allow","Sid":"zever-read"}],"Version":"2012-10-17"}`

	var mu sync.Mutex

	var drifted bool

	storage.SetDecisionHook(func(_ context.Context, d storage.Decision) {
		if d.Reason == storage.ReasonPolicyDrift {
			mu.Lock()
			drifted = true
			mu.Unlock()
		}
	})
	defer storage.SetDecisionHook(nil)

	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, current), nil
	}}
	a := stubAdapter(t, publicReadPolicyConfig(), "require", tr)

	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if drifted {
		t.Error("reordered-but-equal policy reported as drift")
	}
}

// TestEdgeSyncPolicyKeepsForeignStatements preserves non-zever statements in
// the merged document.
func TestEdgeSyncPolicyKeepsForeignStatements(t *testing.T) {
	t.Parallel()

	current := `{"Version":"2012-10-17","Statement":[` +
		`{"Sid":"admin","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::1:root"},` +
		`"Action":"s3:*","Resource":["arn:aws:s3:::b/*"]}]}`

	var merged string

	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPut {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read put body: %v", err)
			}

			merged = string(body)

			return xmlResponse(r, 200, ""), nil
		}

		return xmlResponse(r, 200, current), nil
	}}
	a := stubAdapter(t, publicReadPolicyConfig(), "require", tr)

	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}

	if !strings.Contains(merged, `"admin"`) {
		t.Errorf("merged policy dropped foreign statement: %s", merged)
	}

	if !strings.Contains(merged, `"zever-read"`) {
		t.Errorf("merged policy missing regenerated statement: %s", merged)
	}
}

// TestEdgeSyncPolicyWhitespaceCurrent treats a whitespace-only policy as
// empty.
func TestEdgeSyncPolicyWhitespaceCurrent(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, "   \n "), nil
	}}
	a := stubAdapter(t, privatePolicyConfig(), "require", tr)

	if err := a.SyncPolicy(t.Context(), "b"); err != nil {
		t.Fatalf("SyncPolicy() = %v, want nil", err)
	}

	if tr.calls != 1 {
		t.Fatalf("client calls = %d, want 1 (get only, no put)", tr.calls)
	}
}

// TestEdgeGetBucketPolicyForbidden propagates non-404 error statuses.
func TestEdgeGetBucketPolicyForbidden(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, http.StatusForbidden, ""), nil
	}}
	a := stubAdapter(t, privatePolicyConfig(), "warn", tr)

	if _, err := a.getBucketPolicy(t.Context(), "b"); err == nil {
		t.Fatal("getBucketPolicy() = nil, want error")
	}
}

// TestEdgeMergePolicyWhitespaceCurrent treats a whitespace-only current policy
// as empty.
func TestEdgeMergePolicyWhitespaceCurrent(t *testing.T) {
	t.Parallel()

	a := newPolicyOnlyAdapter(t, privatePolicyConfig())

	merged, drift, err := a.mergePolicy("b", "  \n ")
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

// TestEdgeCanonicalStatementsEmpty covers nil and empty slices.
func TestEdgeCanonicalStatementsEmpty(t *testing.T) {
	t.Parallel()

	if got := canonicalStatements(nil); got != "" {
		t.Errorf("canonicalStatements(nil) = %q, want empty", got)
	}

	if got := canonicalStatements([]jsontext.Value{}); got != "" {
		t.Errorf("canonicalStatements(empty) = %q, want empty", got)
	}
}

// TestEdgeBucketPolicyDocEmptyStatements renders an explicit empty statement
// list for a private policy.
func TestEdgeBucketPolicyDocEmptyStatements(t *testing.T) {
	t.Parallel()

	doc := bucketPolicyDoc(storage.Policy{}, "b")
	if !strings.Contains(doc, `"Statement": []`) {
		t.Fatalf("bucketPolicyDoc() = %s, want empty statements", doc)
	}
}

// newBenchStubAdapter builds an s3Adapter over a scripted transport for
// benchmarks.
func newBenchStubAdapter(b *testing.B, cfg *storage.PolicyConfig, syncFail string, tr *stubTransport) *s3Adapter {
	b.Helper()

	var store storage.PolicyStore
	if err := store.ResolveFromConfig(cfg); err != nil {
		b.Fatal(err)
	}

	awsCfg, err := config.LoadDefaultConfig(b.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("bench-key", "bench-secret", "")),
	)
	if err != nil {
		b.Fatal(err)
	}

	client := s3sdk.NewFromConfig(awsCfg, func(o *s3sdk.Options) {
		o.HTTPClient = tr
		o.Retryer = aws.NopRetryer{}
	})

	return &s3Adapter{
		Core:       s3opts.New("s3", "us-east-1", "", client, nil, store, nil),
		policySync: "auto",
		syncFail:   syncFail,
	}
}
