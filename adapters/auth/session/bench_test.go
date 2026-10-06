package session_test

import (
	"testing"
	"time"
)

// BenchmarkIssue measures session creation plus envelope save.
func BenchmarkIssue(b *testing.B) {
	a := newAdapter(b, newMemoryStore(b))
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.Issue(ctx, "user-1", map[string]any{"role": "admin"}, time.Hour); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerify measures envelope decode and expiry check for a token
// minted once outside the loop.
func BenchmarkVerify(b *testing.B) {
	a := newAdapter(b, newMemoryStore(b))
	ctx := b.Context()

	tok, err := a.Issue(ctx, "user-1", map[string]any{"role": "admin"}, time.Hour)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.Verify(ctx, tok.Value); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRevoke measures token deletion through the store.
func BenchmarkRevoke(b *testing.B) {
	a := newAdapter(b, newMemoryStore(b))
	ctx := b.Context()

	tok, err := a.Issue(ctx, "user-1", nil, time.Hour)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := a.Revoke(ctx, tok.Value); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIssueVerify measures a full issue-then-verify round trip.
func BenchmarkIssueVerify(b *testing.B) {
	a := newAdapter(b, newMemoryStore(b))
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tok, err := a.Issue(ctx, "user-1", nil, time.Hour)
		if err != nil {
			b.Fatal(err)
		}

		if _, err := a.Verify(ctx, tok.Value); err != nil {
			b.Fatal(err)
		}
	}
}
