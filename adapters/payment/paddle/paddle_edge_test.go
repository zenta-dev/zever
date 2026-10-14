package paddle

import (
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/payment"
)

func TestBeginRefundGuard_nilStoreAndEmptyKey(t *testing.T) {
	t.Parallel()

	if ok, err := beginRefundGuard(t.Context(), nil, "k", nil); err != nil || !ok {
		t.Errorf("beginRefundGuard(nil store) = (%v, %v), want (true, nil)", ok, err)
	}

	store := newFakeStore()
	if ok, err := beginRefundGuard(t.Context(), store, "", nil); err != nil || !ok {
		t.Errorf("beginRefundGuard(empty key) = (%v, %v), want (true, nil)", ok, err)
	}

	store.mu.Lock()
	begin := store.begin
	store.mu.Unlock()
	if begin != 0 {
		t.Errorf("Begin calls = %d, want 0 when key empty", begin)
	}
}

func TestBeginRefundGuard_replay(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newFakeStore()
	fp := []byte("fingerprint")

	if _, err := store.Begin(ctx, "k", idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("seed Begin() = %v", err)
	}
	if err := store.Complete(ctx, "k", fp, []byte{1}); err != nil {
		t.Fatalf("seed Complete() = %v", err)
	}

	ok, err := beginRefundGuard(ctx, store, "k", fp)
	if err != nil {
		t.Fatalf("beginRefundGuard() = %v", err)
	}
	if ok {
		t.Error("beginRefundGuard(replay) ok = true, want false")
	}
}

func TestBeginRefundGuard_error(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.failNext = true

	if _, err := beginRefundGuard(t.Context(), store, "k", []byte("fp")); err == nil {
		t.Error("beginRefundGuard(store error) = nil, want error")
	}
}

func TestRefundFingerprint_deterministic(t *testing.T) {
	t.Parallel()

	a := refundFingerprint("txn_1", 100)
	b := refundFingerprint("txn_1", 100)
	if string(a) != string(b) {
		t.Error("refundFingerprint() not deterministic for identical input")
	}
	if string(refundFingerprint("txn_1", 101)) == string(a) {
		t.Error("refundFingerprint() equal across different amounts")
	}
	if string(refundFingerprint("txn_2", 100)) == string(a) {
		t.Error("refundFingerprint() equal across different ids")
	}
}

func TestWebhookEvent_exactLimit_allowed(t *testing.T) {
	t.Parallel()

	const secret = "whsec_test"
	body := []byte(`{"event_type":"transaction.paid","data":{"id":"txn_wh1","status":"paid","currency_code":"USD"}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	p := openTest(t, srv, func(o *payment.Options) { o.MaxWebhookBytes = len(body) })
	if _, err := p.WebhookEvent(t.Context(), body, signWebhook(t, secret, body, time.Now())); err != nil {
		t.Errorf("WebhookEvent(exact limit) = %v, want nil", err)
	}
}

func TestParsePaddleAmount_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"  100.00  ", 10000, false},
		{"0.00", 0, false},
		{"1.", 100, false},
		{"0.5", 50, false},
		{"-0.50", -50, false},
		{"1000000000.99", 100000000099, false},
	}
	for _, tc := range cases {
		got, err := parsePaddleAmount(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parsePaddleAmount(%q) err = nil, want error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("parsePaddleAmount(%q) = (%d, %v), want %d", tc.in, got, err, tc.want)
		}
	}
}

// TestParsePaddleAmount_overflow pins the adversarial-magnitude guard: whole
// parts that parse fine as int64 but no longer fit once scaled to minor units
// (or carry past the limit on the final fraction add) fail with
// ErrAmountOverflow instead of wrapping around, while the exact int64 extrema
// still convert.
func TestParsePaddleAmount_overflow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in           string
		want         int64
		wantErr      bool
		wantOverflow bool
	}{
		{"92233720368547758.07", math.MaxInt64, false, false},
		{"-92233720368547758.08", math.MinInt64, false, false},
		{"-0.99", -99, false, false},
		{"92233720368547758.08", 0, true, true},
		{"92233720368547759.00", 0, true, true},
		{"9223372036854775807.99", 0, true, true},
		{"99999999999999999999.00", 0, true, false},
		{"-92233720368547758.09", 0, true, true},
		{"-92233720368547759.00", 0, true, true},
		{"-9223372036854775808.99", 0, true, true},
	}

	for _, tc := range cases {
		t.Run("amount_"+tc.in, func(t *testing.T) {
			t.Parallel()

			got, err := parsePaddleAmount(tc.in)
			if tc.wantErr {
				if tc.wantOverflow && !errors.Is(err, ErrAmountOverflow) {
					t.Errorf("parsePaddleAmount(%q) err = %v, want ErrAmountOverflow", tc.in, err)
				}

				if err == nil {
					t.Errorf("parsePaddleAmount(%q) err = nil, want error", tc.in)
				}

				return
			}

			if err != nil || got != tc.want {
				t.Errorf("parsePaddleAmount(%q) = (%d, %v), want %d", tc.in, got, err, tc.want)
			}
		})
	}
}

// TestParsePaddleAmount_normalized pins whole-unit to minor-unit scaling,
// round-half-away-from-zero on sub-cent precision, negative symmetry, and
// whole-unit int64 boundaries.
func TestParsePaddleAmount_normalized(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int64
	}{
		{"10", 1000},
		{"0", 0},
		{"-10", -1000},
		{"10.00", 1000},
		{"10.99", 1099},
		{"5.25", 525},
		{"-5.25", -525},
		{"10.004", 1000},
		{"10.005", 1001},
		{"10.995", 1100},
		{"-10.005", -1001},
		{"-10.004", -1000},
		{"0.005", 1},
		{"-0.005", -1},
		{"-99.999", -10000},
		{"92233720368547758", 9223372036854775800},
		{"-92233720368547758", -9223372036854775800},
	}
	for _, tc := range cases {
		t.Run("amount_"+tc.in, func(t *testing.T) {
			t.Parallel()

			got, err := parsePaddleAmount(tc.in)
			if err != nil || got != tc.want {
				t.Errorf("parsePaddleAmount(%q) = (%d, %v), want %d", tc.in, got, err, tc.want)
			}
		})
	}
}

// TestParsePaddleAmount_wholeUnitOverflow pins that whole-unit totals that
// fit as int64 but no longer fit once scaled fail with ErrAmountOverflow,
// while garbage input fails with a plain error.
func TestParsePaddleAmount_wholeUnitOverflow(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"92233720368547759", "-92233720368547759", "9223372036854775807", "-9223372036854775808", "99999999999999999999"} {
		t.Run("amount_"+in, func(t *testing.T) {
			t.Parallel()

			if _, err := parsePaddleAmount(in); !errors.Is(err, ErrAmountOverflow) {
				t.Errorf("parsePaddleAmount(%q) err = %v, want ErrAmountOverflow", in, err)
			}
		})
	}

	for _, in := range []string{"abc", "1e3", "10.0.0", "12.xy", "--10", "10..00"} {
		t.Run("garbage_"+in, func(t *testing.T) {
			t.Parallel()

			if _, err := parsePaddleAmount(in); err == nil {
				t.Errorf("parsePaddleAmount(%q) err = nil, want error", in)
			}
		})
	}
}

// TestFormatPaddleAmount pins the outgoing direction: minor units render as
// major-unit decimals so refund adjustments agree with parsed totals.
func TestFormatPaddleAmount(t *testing.T) {
	t.Parallel()

	cases := map[int64]string{
		0:             "0.00",
		1:             "0.01",
		50:            "0.50",
		1000:          "10.00",
		1099:          "10.99",
		-525:          "-5.25",
		math.MaxInt64: "92233720368547758.07",
		math.MinInt64: "-92233720368547758.08",
	}

	for in, want := range cases {
		if got := formatPaddleAmount(in); got != want {
			t.Errorf("formatPaddleAmount(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestPaddleAmountRoundTrip pins that two-decimal totals survive a
// parse/format round trip.
func TestPaddleAmountRoundTrip(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"25.00", "10.99", "0.01", "-5.25", "1000000000.99"} {
		minor, err := parsePaddleAmount(s)
		if err != nil {
			t.Fatalf("parsePaddleAmount(%q) = %v", s, err)
		}

		if back, err := parsePaddleAmount(formatPaddleAmount(minor)); err != nil || back != minor {
			t.Errorf("round trip(%q): parse(format(%d)) = (%d, %v), want (%d, nil)", s, minor, back, err, minor)
		}
	}
}

func TestClose_idempotent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	p := openTest(t, srv, nil)
	if err := p.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}
