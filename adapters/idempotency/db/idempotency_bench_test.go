package db

import (
	"fmt"
	"path/filepath"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/idempotency"
)

// benchStore opens a file-backed sqlite store for one benchmark and closes
// it on cleanup. A file path avoids the process-global shared cache of
// ":memory:", keeping parallel benchmarks on distinct databases.
func benchStore(b *testing.B) idempotency.Store {
	b.Helper()

	s, err := New(Options{Options: coredb.Options{Path: filepath.Join(b.TempDir(), "idem.db")}})
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	return s
}

// benchKey returns a key unique to n so the reservation-miss path runs on
// every iteration instead of replaying a stored record.
func benchKey(n int) string {
	return fmt.Sprintf("bench-%d", n)
}

// BenchmarkNew measures opening a file-backed sqlite store (schema ensure)
// and closing it.
func BenchmarkNew(b *testing.B) {
	dir := b.TempDir()

	b.ReportAllocs()

	for b.Loop() {
		s, err := New(Options{Options: coredb.Options{Path: filepath.Join(dir, "idem.db")}})
		if err != nil {
			b.Fatalf("New failed: %v", err)
		}

		if err := s.Close(); err != nil {
			b.Fatalf("Close failed: %v", err)
		}
	}
}

// BenchmarkBeginMiss measures Begin on a fresh key: the SET NX claim wins.
func BenchmarkBeginMiss(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()
	fp := []byte("fp")

	n := 0

	b.ReportAllocs()

	for b.Loop() {
		n++

		if _, err := s.Begin(ctx, benchKey(n), idempotency.BeginOptions{Fingerprint: fp}); err != nil {
			b.Fatalf("Begin failed: %v", err)
		}
	}
}

// BenchmarkBeginReplay measures Begin against a completed record: the read
// path decodes the wire record and returns the stored result.
func BenchmarkBeginReplay(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()
	fp := []byte("fp")
	key := benchKey(0)

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		b.Fatalf("seed Begin failed: %v", err)
	}

	if err := s.Complete(ctx, key, fp, []byte("result")); err != nil {
		b.Fatalf("seed Complete failed: %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
		if err != nil {
			b.Fatalf("Begin failed: %v", err)
		}

		if !out.Replay {
			b.Fatal("Begin Replay = false, want true")
		}
	}
}

// BenchmarkComplete measures Complete on a missing key: the read-then-upsert
// path.
func BenchmarkComplete(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()
	fp := []byte("fp")

	n := 0

	b.ReportAllocs()

	for b.Loop() {
		n++

		if err := s.Complete(ctx, benchKey(n), fp, []byte("result")); err != nil {
			b.Fatalf("Complete failed: %v", err)
		}
	}
}

// BenchmarkForget measures Forget, which deletes the kv row.
func BenchmarkForget(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()
	key := benchKey(0)

	b.ReportAllocs()

	for b.Loop() {
		if err := s.Forget(ctx, key); err != nil {
			b.Fatalf("Forget failed: %v", err)
		}
	}
}

// BenchmarkEncodePending measures the pending wire-record builder.
func BenchmarkEncodePending(b *testing.B) {
	fp := []byte("fingerprint")

	b.ReportAllocs()

	for b.Loop() {
		_ = encodePending(fp)
	}
}

// BenchmarkEncodeDone measures the completed wire-record builder.
func BenchmarkEncodeDone(b *testing.B) {
	fp := []byte("fingerprint")
	result := []byte("result-payload")

	b.ReportAllocs()

	for b.Loop() {
		_ = encodeDone(fp, result)
	}
}

// BenchmarkDecode measures splitting a completed wire record.
func BenchmarkDecode(b *testing.B) {
	raw := encodeDone([]byte("fingerprint"), []byte("result-payload"))

	b.ReportAllocs()

	for b.Loop() {
		if _, _, _, err := decode(raw); err != nil {
			b.Fatalf("decode failed: %v", err)
		}
	}
}

// BenchmarkCheckFingerprint measures the fingerprint size guard.
func BenchmarkCheckFingerprint(b *testing.B) {
	fp := []byte("fingerprint")

	b.ReportAllocs()

	for b.Loop() {
		if err := checkFingerprint(fp); err != nil {
			b.Fatalf("checkFingerprint failed: %v", err)
		}
	}
}

// BenchmarkKVKey measures prefixing an idempotency key.
func BenchmarkKVKey(b *testing.B) {
	d := &driver{prefix: "idem:"}

	b.ReportAllocs()

	for b.Loop() {
		_ = d.kvKey("bench-key")
	}
}
