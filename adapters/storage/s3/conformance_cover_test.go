package s3

import (
	"testing"

	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/storage/storagetest"
)

// TestS3Conformance wires the s3 adapter into the shared storage
// conformance kit.
//
// Currently skipped: the adapter needs live S3 credentials
// (endpoint + keys) and network access. The kit's mint-shape and
// validation assertions match the local adapter's; provider mapping
// coverage lives in the adapter's own tests. Re-enable with test
// credentials.
func TestS3Conformance(t *testing.T) {
	t.Skip("needs live S3 credentials and network")

	storagetest.Conformance(t, func(t *testing.T) storage.Storage {
		t.Helper()

		s, err := New(storage.Options{URLBase: "https://cdn.example.com/base"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close(t.Context()) })

		return s
	})
}
