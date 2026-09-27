// Package storagetest provides the conformance kit third-party storage adapters run to prove backend parity.
package storagetest

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

// DefaultPresignTTL is the lifetime conformance presign tests mint URLs with.
const DefaultPresignTTL = time.Minute

// Conformance verifies factory-built storages implement the storage.Storage
// contract: presigned upload/download minting and validation, Exists, Delete,
// Move, and Close. Each subtest takes a fresh instance from factory so cases
// stay isolated. The kit never touches the network: the write path lives
// outside the Storage interface (HTTP PUT to the minted URL), so positive
// object lifecycle (Exists true, Move of an existing object) is the proof
// test's job to seed through backend-specific means. The kit covers every
// behavior reachable through the interface alone: mint shape, validation
// failures, absent-object behavior, self-move, and Close.
func Conformance(t *testing.T, factory func(t *testing.T) storage.Storage) {
	t.Helper()

	t.Run("PresignUpload", func(t *testing.T) { conformancePresignUpload(t, factory) })
	t.Run("PresignDownload", func(t *testing.T) { conformancePresignDownload(t, factory) })
	t.Run("Exists", func(t *testing.T) { conformanceExists(t, factory) })
	t.Run("Delete", func(t *testing.T) { conformanceDelete(t, factory) })
	t.Run("Move", func(t *testing.T) { conformanceMove(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformancePresignUpload(t *testing.T, factory func(t *testing.T) storage.Storage) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	got, err := s.PresignUpload(ctx, "kit-upload", "a/b.txt", "text/plain", DefaultPresignTTL)
	if err != nil {
		t.Fatalf("PresignUpload() error = %v", err)
	}

	if got.Method != storage.HTTPMethodPUT {
		t.Errorf("PresignUpload() Method = %q, want PUT", got.Method)
	}

	if got.URL == "" {
		t.Error("PresignUpload() URL is empty")
	} else {
		if !strings.Contains(got.URL, "kit-upload") {
			t.Errorf("PresignUpload() URL = %q, want it to mention the bucket", got.URL)
		}

		if !strings.Contains(got.URL, "a/b.txt") && !strings.Contains(got.URL, "a%2Fb.txt") {
			t.Errorf("PresignUpload() URL = %q, want it to mention the key", got.URL)
		}
	}

	cases := []struct {
		name   string
		bucket string
		key    string
		ttl    time.Duration
		want   error
	}{
		{"empty bucket", "", "a/b.txt", DefaultPresignTTL, storage.ErrInvalidBucket},
		{"bad bucket", "bad/name", "a/b.txt", DefaultPresignTTL, storage.ErrInvalidBucket},
		{"empty key", "kit-upload", "", DefaultPresignTTL, storage.ErrInvalidKey},
		{"dotdot key", "kit-upload", "a/../b", DefaultPresignTTL, storage.ErrInvalidKey},
		{"zero ttl", "kit-upload", "a/b.txt", 0, storage.ErrExpired},
		{"negative ttl", "kit-upload", "a/b.txt", -time.Second, storage.ErrExpired},
	}

	for _, tc := range cases {
		_, err := s.PresignUpload(ctx, tc.bucket, tc.key, "text/plain", tc.ttl)
		if !errors.Is(err, tc.want) {
			t.Errorf("PresignUpload(%s) err = %v, want %v", tc.name, err, tc.want)
		}
	}

	if _, err := s.PresignUpload(ctx, "kit-upload", "a/b.txt", "text/plain", storage.MaxPresignTTL+time.Second); err == nil {
		t.Error("PresignUpload(oversize ttl) = nil, want error")
	}
}

func conformancePresignDownload(t *testing.T, factory func(t *testing.T) storage.Storage) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	got, err := s.PresignDownload(ctx, "kit-download", "a/b.txt", DefaultPresignTTL)
	if err != nil {
		t.Fatalf("PresignDownload() error = %v", err)
	}

	if got.Method != storage.HTTPMethodGET {
		t.Errorf("PresignDownload() Method = %q, want GET", got.Method)
	}

	if got.URL == "" {
		t.Error("PresignDownload() URL is empty")
	} else {
		if !strings.Contains(got.URL, "kit-download") {
			t.Errorf("PresignDownload() URL = %q, want it to mention the bucket", got.URL)
		}

		if !strings.Contains(got.URL, "a/b.txt") && !strings.Contains(got.URL, "a%2Fb.txt") {
			t.Errorf("PresignDownload() URL = %q, want it to mention the key", got.URL)
		}
	}

	cases := []struct {
		name   string
		bucket string
		key    string
		ttl    time.Duration
		want   error
	}{
		{"empty bucket", "", "a/b.txt", DefaultPresignTTL, storage.ErrInvalidBucket},
		{"bad bucket", "bad/name", "a/b.txt", DefaultPresignTTL, storage.ErrInvalidBucket},
		{"empty key", "kit-download", "", DefaultPresignTTL, storage.ErrInvalidKey},
		{"dotdot key", "kit-download", "a/../b", DefaultPresignTTL, storage.ErrInvalidKey},
		{"zero ttl", "kit-download", "a/b.txt", 0, storage.ErrExpired},
		{"negative ttl", "kit-download", "a/b.txt", -time.Second, storage.ErrExpired},
	}

	for _, tc := range cases {
		_, err := s.PresignDownload(ctx, tc.bucket, tc.key, tc.ttl)
		if !errors.Is(err, tc.want) {
			t.Errorf("PresignDownload(%s) err = %v, want %v", tc.name, err, tc.want)
		}
	}

	if _, err := s.PresignDownload(ctx, "kit-download", "a/b.txt", storage.MaxPresignTTL+time.Second); err == nil {
		t.Error("PresignDownload(oversize ttl) = nil, want error")
	}
}

func conformanceExists(t *testing.T, factory func(t *testing.T) storage.Storage) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	ok, err := s.Exists(ctx, "kit-exists", "missing.txt")
	if err != nil || ok {
		t.Fatalf("Exists(missing) = %v,%v want false,nil", ok, err)
	}

	if _, err := s.Exists(ctx, "", "a.txt"); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Errorf("Exists(empty bucket) err = %v, want ErrInvalidBucket", err)
	}

	if _, err := s.Exists(ctx, "kit-exists", ""); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("Exists(empty key) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceDelete(t *testing.T, factory func(t *testing.T) storage.Storage) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	// Deleting a missing object is a no-op.
	if err := s.Delete(ctx, "kit-delete", "missing.txt"); err != nil {
		t.Fatalf("Delete(missing) error = %v, want nil", err)
	}

	if err := s.Delete(ctx, "", "a.txt"); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Errorf("Delete(empty bucket) err = %v, want ErrInvalidBucket", err)
	}

	if err := s.Delete(ctx, "kit-delete", ""); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("Delete(empty key) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceMove(t *testing.T, factory func(t *testing.T) storage.Storage) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	// Moving a key onto itself is a no-op, even when absent.
	if err := s.Move(ctx, "kit-move", "same.txt", "kit-move", "same.txt"); err != nil {
		t.Errorf("Move(self) error = %v, want nil", err)
	}

	if err := s.Move(ctx, "kit-move", "missing.txt", "kit-move", "dst.txt"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Move(missing source) err = %v, want ErrNotFound", err)
	}

	if err := s.Move(ctx, "", "a.txt", "kit-move", "b.txt"); !errors.Is(err, storage.ErrInvalidBucket) {
		t.Errorf("Move(empty src bucket) err = %v, want ErrInvalidBucket", err)
	}

	if err := s.Move(ctx, "kit-move", "a.txt", "kit-move", ""); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("Move(empty dst key) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) storage.Storage) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if s.Name() == "" {
		t.Error("Name() is empty, want adapter name")
	}

	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(ctx); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
