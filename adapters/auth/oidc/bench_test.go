package oidc_test

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/auth/oidc"
	"github.com/zenta-dev/zever/core/auth"
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

	for b.Loop() {
		if _, err := a.Verify(ctx, token); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNew measures the construction path: options validation plus OIDC
// discovery against the hermetic provider.
func BenchmarkNew(b *testing.B) {
	idp := newFakeIDP(b)

	opts := auth.Options{}
	opts.OIDC.Issuer = idp.srv.URL
	opts.OIDC.ClientID = "test-client"
	opts.OIDC.AllowInsecure = true

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		a, err := oidc.New(opts)
		if err != nil {
			b.Fatal(err)
		}
		if err := a.Close(); err != nil {
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
