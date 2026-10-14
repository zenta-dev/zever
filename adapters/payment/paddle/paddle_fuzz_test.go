package paddle

import (
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

// mustTestDriver builds an offline driver: signature verification is pure
// HMAC over the body, so no network is involved.
func mustTestDriver(t *testing.T) *driver {
	t.Helper()

	p, err := New(payment.Options{APIKey: "pdl_api_test", WebhookSecret: "whsec_test"}) //nolint:gosec // synthetic test credential
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	d, ok := p.(*driver)
	if !ok {
		t.Fatalf("New() type = %T, want *driver", p)
	}

	return d
}

// FuzzWebhookEvent proves verification never panics on arbitrary bodies
// and signatures: every input yields an event or a clean error.
// Well-formed bodies with bad signatures must fail closed.
func FuzzWebhookEvent(f *testing.F) {
	for _, seed := range []struct{ raw, sig string }{
		{"", ""},
		{"{}", ""},
		{`{"event_type":"transaction.completed"}`, "ts=123;h1=deadbeef"},
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

// FuzzParsePaddleAmount proves amount parsing never panics on arbitrary
// input: every string yields minor units or a clean error. The parser is
// deliberately signed (negative inputs parse); webhook totals are never
// negative in practice and signature verification rejects forged bodies.
func FuzzParsePaddleAmount(f *testing.F) {
	for _, seed := range []string{"", "0", "10.00", "7.5", "abc", "-3.5", "1e3", "10.000", ".5"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		t.Parallel()

		_, _ = parsePaddleAmount(s, "USD")
	})
}

// TestParsePaddleAmountKnown pins the decimal-to-minor-units contract.
// Totals are major-unit decimals: bare whole units scale x100 ("10" is a
// thousand minor units). The grammar is strict: a present point requires
// digits on both sides.
func TestParsePaddleAmountKnown(t *testing.T) {
	t.Parallel()

	cases := map[string]int64{
		"":      0,
		"0":     0,
		"10":    1000,
		"10.00": 1000,
		"7.5":   750,
		"-3.5":  -350,
	}

	for in, want := range cases {
		if got, err := parsePaddleAmount(in, "USD"); err != nil || got != want {
			t.Errorf("parsePaddleAmount(%q) = (%d, %v), want (%d, nil)", in, got, err, want)
		}
	}

	for _, bad := range []string{"abc", "1e3", "10.0.0", ".5", "1."} {
		if got, err := parsePaddleAmount(bad, "USD"); err == nil {
			t.Errorf("parsePaddleAmount(%q) = %d, want error", bad, got)
		}
	}
}
