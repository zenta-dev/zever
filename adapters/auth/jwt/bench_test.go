package jwt

import (
	"testing"
	"time"
)

// BenchmarkIssue measures HS256 token minting.
func BenchmarkIssue(b *testing.B) {
	a := freshAdapter(b, baseOpts())
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.Issue(ctx, "user-1", map[string]any{"role": "admin"}, time.Hour); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerify measures signature verification and claim extraction for a
// token minted once outside the loop.
func BenchmarkVerify(b *testing.B) {
	a := freshAdapter(b, baseOpts())
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

// BenchmarkIssueVerify measures a full mint-then-verify round trip.
func BenchmarkIssueVerify(b *testing.B) {
	a := freshAdapter(b, baseOpts())
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

// BenchmarkRevoke measures signature-only parsing plus revocation-store write.
func BenchmarkRevoke(b *testing.B) {
	a := freshAdapter(b, baseOpts())
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

// BenchmarkVerifyParallel measures verification throughput under concurrent
// load; the adapter and its revocation store are goroutine-safe.
func BenchmarkVerifyParallel(b *testing.B) {
	a := freshAdapter(b, baseOpts())
	ctx := b.Context()

	tok, err := a.Issue(ctx, "user-1", nil, time.Hour)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := a.Verify(ctx, tok.Value); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
