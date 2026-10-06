// Package storagetest provides the conformance kit third-party storage adapters run to prove backend parity.
package storagetest

import (
	"context"
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
// reporter is the *testing.T subset the conformance checks need. The kit
// passes the real *testing.T; edge tests pass a recording stub to drive
// failure branches without failing the suite.
type reporter interface {
	Helper()
	Context() context.Context
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

func Conformance(t *testing.T, factory func(t *testing.T) storage.Storage) {
	t.Helper()

	t.Run("PresignUpload", func(t *testing.T) { conformancePresignUpload(t, factory(t)) })
	t.Run("PresignDownload", func(t *testing.T) { conformancePresignDownload(t, factory(t)) })
	t.Run("Exists", func(t *testing.T) { conformanceExists(t, factory(t)) })
	t.Run("Delete", func(t *testing.T) { conformanceDelete(t, factory(t)) })
	t.Run("Move", func(t *testing.T) { conformanceMove(t, factory(t)) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory(t)) })
}

func conformancePresignUpload(r reporter, s storage.Storage) {
	r.Helper()

	ctx := r.Context()

	got, err := s.PresignUpload(ctx, "kit-upload", "a/b.txt", "text/plain", DefaultPresignTTL)
	if err != nil {
		r.Fatalf("PresignUpload() error = %v", err)
	}

	if got.Method != storage.HTTPMethodPUT {
		r.Errorf("PresignUpload() Method = %q, want PUT", got.Method)
	}

	if got.URL == "" {
		r.Error("PresignUpload() URL is empty")
	} else {
		if !strings.Contains(got.URL, "kit-upload") {
			r.Errorf("PresignUpload() URL = %q, want it to mention the bucket", got.URL)
		}

		if !strings.Contains(got.URL, "a/b.txt") && !strings.Contains(got.URL, "a%2Fb.txt") {
			r.Errorf("PresignUpload() URL = %q, want it to mention the key", got.URL)
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
			r.Errorf("PresignUpload(%s) err = %v, want %v", tc.name, err, tc.want)
		}
	}

	if _, err := s.PresignUpload(ctx, "kit-upload", "a/b.txt", "text/plain", storage.MaxPresignTTL+time.Second); err == nil {
		r.Error("PresignUpload(oversize ttl) = nil, want error")
	}
}

func conformancePresignDownload(r reporter, s storage.Storage) {
	r.Helper()

	ctx := r.Context()

	got, err := s.PresignDownload(ctx, "kit-download", "a/b.txt", DefaultPresignTTL)
	if err != nil {
		r.Fatalf("PresignDownload() error = %v", err)
	}

	if got.Method != storage.HTTPMethodGET {
		r.Errorf("PresignDownload() Method = %q, want GET", got.Method)
	}

	if got.URL == "" {
		r.Error("PresignDownload() URL is empty")
	} else {
		if !strings.Contains(got.URL, "kit-download") {
			r.Errorf("PresignDownload() URL = %q, want it to mention the bucket", got.URL)
		}

		if !strings.Contains(got.URL, "a/b.txt") && !strings.Contains(got.URL, "a%2Fb.txt") {
			r.Errorf("PresignDownload() URL = %q, want it to mention the key", got.URL)
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
			r.Errorf("PresignDownload(%s) err = %v, want %v", tc.name, err, tc.want)
		}
	}

	if _, err := s.PresignDownload(ctx, "kit-download", "a/b.txt", storage.MaxPresignTTL+time.Second); err == nil {
		r.Error("PresignDownload(oversize ttl) = nil, want error")
	}
}

func conformanceExists(r reporter, s storage.Storage) {
	r.Helper()

	ctx := r.Context()

	ok, err := s.Exists(ctx, "kit-exists", "missing.txt")
	if err != nil || ok {
		r.Fatalf("Exists(missing) = %v,%v want false,nil", ok, err)
	}

	if _, err := s.Exists(ctx, "", "a.txt"); !errors.Is(err, storage.ErrInvalidBucket) {
		r.Errorf("Exists(empty bucket) err = %v, want ErrInvalidBucket", err)
	}

	if _, err := s.Exists(ctx, "kit-exists", ""); !errors.Is(err, storage.ErrInvalidKey) {
		r.Errorf("Exists(empty key) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceDelete(r reporter, s storage.Storage) {
	r.Helper()

	ctx := r.Context()

	// Deleting a missing object is a no-op.
	if err := s.Delete(ctx, "kit-delete", "missing.txt"); err != nil {
		r.Fatalf("Delete(missing) error = %v, want nil", err)
	}

	if err := s.Delete(ctx, "", "a.txt"); !errors.Is(err, storage.ErrInvalidBucket) {
		r.Errorf("Delete(empty bucket) err = %v, want ErrInvalidBucket", err)
	}

	if err := s.Delete(ctx, "kit-delete", ""); !errors.Is(err, storage.ErrInvalidKey) {
		r.Errorf("Delete(empty key) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceMove(r reporter, s storage.Storage) {
	r.Helper()

	ctx := r.Context()

	// Moving a key onto itself is a no-op, even when absent.
	if err := s.Move(ctx, "kit-move", "same.txt", "kit-move", "same.txt"); err != nil {
		r.Errorf("Move(self) error = %v, want nil", err)
	}

	if err := s.Move(ctx, "kit-move", "missing.txt", "kit-move", "dst.txt"); !errors.Is(err, storage.ErrNotFound) {
		r.Errorf("Move(missing source) err = %v, want ErrNotFound", err)
	}

	if err := s.Move(ctx, "", "a.txt", "kit-move", "b.txt"); !errors.Is(err, storage.ErrInvalidBucket) {
		r.Errorf("Move(empty src bucket) err = %v, want ErrInvalidBucket", err)
	}

	if err := s.Move(ctx, "kit-move", "a.txt", "kit-move", ""); !errors.Is(err, storage.ErrInvalidKey) {
		r.Errorf("Move(empty dst key) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceClose(r reporter, s storage.Storage) {
	r.Helper()

	ctx := r.Context()

	if s.Name() == "" {
		r.Error("Name() is empty, want adapter name")
	}

	if err := s.Close(ctx); err != nil {
		r.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(ctx); err != nil {
		r.Errorf("Close() second error = %v, want nil", err)
	}
}
