package storagetest_test

import (
	"os"
	"path/filepath"
	"testing"

	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/storage/storagetest"
)

const testSecret = "test-secret-0123456789abcdef"

// TestConformanceLocal proves the kit passes against the local adapter.
func TestConformanceLocal(t *testing.T) {
	t.Parallel()

	storagetest.Conformance(t, func(t *testing.T) storage.Storage {
		t.Helper()

		s, err := storagelocal.New(storage.Options{
			URLBase:      "https://example.com",
			LocalOptions: storage.LocalOptions{Root: t.TempDir(), Secret: testSecret},
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = s.Close(t.Context()) })

		return s
	})
}

// TestLocalSeededLifecycle proves the positive object lifecycle the kit
// cannot seed through the interface alone: a backend-side object is visible
// to Exists, Move relocates it, and Delete removes it.
func TestLocalSeededLifecycle(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	root := t.TempDir()

	s, err := storagelocal.New(storage.Options{
		URLBase:      "https://example.com",
		LocalOptions: storage.LocalOptions{Root: root, Secret: testSecret},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = s.Close(t.Context()) })

	const bucket = "kit-seed"

	full := filepath.Join(root, bucket, "src.txt")
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(full, []byte("seed"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if ok, err := s.Exists(ctx, bucket, "src.txt"); err != nil || !ok {
		t.Fatalf("Exists(seeded) = %v,%v want true,nil", ok, err)
	}

	if err := s.Move(ctx, bucket, "src.txt", bucket, "dst.txt"); err != nil {
		t.Fatalf("Move() error = %v", err)
	}

	if ok, err := s.Exists(ctx, bucket, "src.txt"); err != nil || ok {
		t.Fatalf("Exists(src after move) = %v,%v want false,nil", ok, err)
	}

	if ok, err := s.Exists(ctx, bucket, "dst.txt"); err != nil || !ok {
		t.Fatalf("Exists(dst after move) = %v,%v want true,nil", ok, err)
	}

	if err := s.Delete(ctx, bucket, "dst.txt"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if ok, err := s.Exists(ctx, bucket, "dst.txt"); err != nil || ok {
		t.Fatalf("Exists(after delete) = %v,%v want false,nil", ok, err)
	}
}
