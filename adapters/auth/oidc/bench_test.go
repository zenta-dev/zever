package oidc_test

import (
	"testing"
	"time"
)

// BenchmarkVerify measures ID-token signature verification and claim
// extraction against a hermetic in-process provider.
func BenchmarkVerify(b *testing.B) {
	idp := newFakeIDP(b)
	a := newAdapter(b, idp)
	ctx := b.Context()

	token := mintToken(b, idp.key, idp.kid, idp.srv.URL, "test-client", time.Now().Add(time.Hour), nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := a.Verify(ctx, token); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifyParallel measures verification throughput under concurrent
// load; the oidc verifier is safe for concurrent use.
func BenchmarkVerifyParallel(b *testing.B) {
	idp := newFakeIDP(b)
	a := newAdapter(b, idp)
	ctx := b.Context()

	token := mintToken(b, idp.key, idp.kid, idp.srv.URL, "test-client", time.Now().Add(time.Hour), nil)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := a.Verify(ctx, token); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
