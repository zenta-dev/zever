package storage

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

var benchStorageSeq atomic.Int64

func benchStorageAdapter() Adapter {
	return Adapter(fmt.Sprintf("bench-%d", 4000+int(benchStorageSeq.Add(1))))
}

func BenchmarkOpen(b *testing.B) {
	a := benchStorageAdapter()
	if err := Register(a, func(Options) (Storage, error) { return stubStorage{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkPolicyAllow(b *testing.B) {
	pol := Policy{
		Read: Rule{Allow: []Subject{"alice", "bob"}, Deny: []Subject{"mallory"}},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !pol.Allow(PermRead, "alice", true) {
			b.Fatal("Allow(alice, read) = false")
		}
	}
}

func BenchmarkValidKey(b *testing.B) {
	key := "tenant/2026/10/report-final.pdf"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !ValidKey(key) {
			b.Fatalf("ValidKey(%q) = false", key)
		}
	}
}

func BenchmarkValidBucket(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !ValidBucket("my-bucket.prod") {
			b.Fatal("ValidBucket = false")
		}
	}
}

func BenchmarkPresignExpiry(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := PresignExpiry(time.Hour); err != nil {
			b.Fatalf("PresignExpiry error = %v", err)
		}
	}
}

func BenchmarkValidateBucketKey(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := ValidateBucketKey("mybucket", "a/b.txt"); err != nil {
			b.Fatalf("ValidateBucketKey error = %v", err)
		}
	}
}

func BenchmarkSelectUploadPerm(b *testing.B) {
	pol := Policy{Write: Rule{Allow: []Subject{"writer"}}, Update: Rule{Allow: []Subject{"updater"}}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if perm := SelectUploadPerm(pol, true); perm != PermUpdate {
			b.Fatalf("SelectUploadPerm = %q, want update", perm)
		}
	}
}
