package s3

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/storage/storagetest"
)

// liveString returns the value of env, or fallback when unset or blank.
func liveString(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}

	return fallback
}

// TestS3ConformanceLive runs the shared storage conformance kit against a
// real S3-compatible server (e.g. minio in the conformance-live CI
// workflow). Skipped unless ZEVER_S3_ENDPOINT names the server, so local
// runs without docker stay green. Stub-transport coverage lives in
// s3_test.go; never fake infra for conformance.
func TestS3ConformanceLive(t *testing.T) {
	endpoint := os.Getenv("ZEVER_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set ZEVER_S3_ENDPOINT to run storage conformance against a live S3-compatible server")
	}

	urlBase := liveString("ZEVER_S3_URL_BASE", endpoint)

	storagetest.Conformance(t, func(t *testing.T) storage.Storage {
		t.Helper()

		a, err := New(storage.Options{
			URLBase: urlBase,
			S3Options: storage.S3Options{
				Endpoint:        endpoint,
				Region:          liveString("ZEVER_S3_REGION", "us-east-1"),
				AccessKeyID:     liveString("ZEVER_S3_ACCESS_KEY_ID", "minioadmin"),
				SecretAccessKey: liveString("ZEVER_S3_SECRET_ACCESS_KEY", "minioadmin"),
			},
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return a
	})
}
