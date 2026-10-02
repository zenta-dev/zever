package local

import (
	"testing"

	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/storage/storagetest"
)

// TestLocalConformance wires the local adapter into the shared
// storage conformance kit. Each subtest gets a fresh temp-dir-backed
// instance (no network).
func TestLocalConformance(t *testing.T) {
	storagetest.Conformance(t, func(t *testing.T) storage.Storage {
		t.Helper()

		s, err := New(storage.Options{
			URLBase:      "https://cdn.example.com/base",
			LocalOptions: storage.LocalOptions{Root: t.TempDir(), Secret: "test-secret-0123456789abcdef"},
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close(t.Context()) })

		return s
	})
}
