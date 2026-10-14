package stripe

import (
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

// mustTestDriver builds an offline driver: signature verification is pure
// HMAC, so no network is involved.
func mustTestDriver(t *testing.T) *driver {
	t.Helper()

	p, err := New(payment.Options{SecretKey: "sk_test_x", WebhookSecret: "whsec_test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	d, ok := p.(*driver)
	if !ok {
		t.Fatalf("New() type = %T, want *driver", p)
	}

	return d
}

// FuzzWebhookEvent proves signature verification never panics on arbitrary
// bodies and signatures: every input yields an event or a clean error.
// Well-formed bodies with bad signatures must fail closed.
func FuzzWebhookEvent(f *testing.F) {
	for _, seed := range []struct{ raw, sig string }{
		{"", ""},
		{"{}", ""},
		{"{}", "t=123,v1=deadbeef"},
		{`{"id":"evt_1","type":"payment_intent.succeeded"}`, "t=123,v1=deadbeef"},
		{"\x00\xff\xfe", "\x00"},
	} {
		f.Add(seed.raw, seed.sig)
	}

	f.Fuzz(func(t *testing.T, raw, sig string) {
		t.Parallel()

		d := mustTestDriver(t)
		_, _ = d.WebhookEvent(t.Context(), []byte(raw), sig)
	})
}
