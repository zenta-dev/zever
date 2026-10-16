package s3

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/storage/storagetest"
)

// liveBuckets are the buckets the storage kit addresses. Real S3 (and
// S3-compatible servers) answer DeleteObject on a MISSING bucket with
// NoSuchBucket, while the kit defines deleting a missing object as a no-op —
// so the live server must have the buckets, exactly like production.
var liveBuckets = []string{"kit-upload", "kit-download", "kit-exists", "kit-delete", "kit-move"}

// ensureLiveBuckets creates each kit bucket, tolerating already-exists: the
// server persists across kit subtests (and across workflow reruns), so the
// second factory call must not fail.
func ensureLiveBuckets(t *testing.T, ctx context.Context, endpoint, region, key, secret string) {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(key, secret, "")),
	)
	if err != nil {
		t.Fatalf("LoadDefaultConfig() error = %v", err)
	}

	cl := s3sdk.NewFromConfig(cfg, func(o *s3sdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	for _, b := range liveBuckets {
		_, err := cl.CreateBucket(ctx, &s3sdk.CreateBucketInput{Bucket: aws.String(b)})
		if err == nil {
			continue
		}

		var apiErr interface{ ErrorCode() string }
		if errors.As(err, &apiErr) &&
			(apiErr.ErrorCode() == "BucketAlreadyOwnedByYou" || apiErr.ErrorCode() == "BucketAlreadyExists") {
			continue
		}

		t.Fatalf("CreateBucket(%q) error = %v", b, err)
	}
}
func liveString(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}

	return fallback
}

// TestS3ConformanceLive runs the shared storage conformance kit against a
// real S3-compatible server (Adobe S3Mock in the conformance-live CI
// workflow). Skipped unless ZEVER_S3_ENDPOINT names the server, so local
// runs without docker stay green. Stub-transport coverage lives in
// s3_test.go; never fake infra for conformance.
func TestS3ConformanceLive(t *testing.T) {
	endpoint := os.Getenv("ZEVER_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set ZEVER_S3_ENDPOINT to run storage conformance against a live S3-compatible server")
	}

	urlBase := liveString("ZEVER_S3_URL_BASE", endpoint)
	region := liveString("ZEVER_S3_REGION", "us-east-1")
	accessKey := liveString("ZEVER_S3_ACCESS_KEY_ID", "minioadmin")
	secretKey := liveString("ZEVER_S3_SECRET_ACCESS_KEY", "minioadmin")

	ensureLiveBuckets(t, t.Context(), endpoint, region, accessKey, secretKey)

	storagetest.Conformance(t, func(t *testing.T) storage.Storage {
		t.Helper()

		a, err := New(storage.Options{
			URLBase: urlBase,
			S3Options: storage.S3Options{
				Endpoint:        endpoint,
				Region:          region,
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
			},
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return a
	})
}
