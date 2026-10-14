package paddle

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/payment"
)

// TestParsePaddleAmountMinorUnits pins the uniform minor-units contract:
// Paddle decimal totals are major units, so whole units scale x100, excess
// precision rounds half away from zero, and unrepresentable magnitudes fail
// with the shared overflow sentinel.
func TestParsePaddleAmountMinorUnits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		currency string
		want     int64
	}{
		{"whole scales", "10", "USD", 1000},
		{"decimal", "100.00", "USD", 10000},
		{"cents", "99.99", "USD", 9999},
		{"half away positive", "99.999", "USD", 10000},
		{"half away negative", "-99.999", "USD", -10000},
		{"negative whole", "-5", "USD", -500},
		{"negative decimal", "-5.25", "USD", -525},
		{"single decimal", "1.5", "USD", 150},
		{"empty maps zero", "", "USD", 0},
		{"spaces trim", "  100.00  ", "USD", 10000},
		{"jpy whole", "100", "JPY", 100},
		{"jpy fraction rounds", "100.5", "JPY", 101},
		{"max boundary", "92233720368547758.07", "USD", math.MaxInt64},
		{"min boundary", "-92233720368547758.08", "USD", math.MinInt64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePaddleAmount(tc.in, tc.currency)
			if err != nil {
				t.Fatalf("parsePaddleAmount(%q, %q) err = %v", tc.in, tc.currency, err)
			}

			if got != tc.want {
				t.Errorf("parsePaddleAmount(%q, %q) = %d, want %d", tc.in, tc.currency, got, tc.want)
			}
		})
	}
}

func TestParsePaddleAmountMinorUnits_errors(t *testing.T) {
	t.Parallel()

	garbage := []struct{ in, currency string }{
		{"abc", "USD"},
		{"12.xy", "USD"},
		{"xx.00", "USD"},
		{"1e3", "USD"},
		{"10.0.0", "USD"},
		{".99", "USD"},
		{"1.", "USD"},
	}
	for _, tc := range garbage {
		if _, err := parsePaddleAmount(tc.in, tc.currency); err == nil {
			t.Errorf("parsePaddleAmount(%q) err = nil, want error", tc.in)
		}
	}

	overflow := []struct{ in, currency string }{
		{"92233720368547758.08", "USD"},
		{"92233720368547759", "USD"},
		{"-92233720368547758.09", "USD"},
	}
	for _, tc := range overflow {
		_, err := parsePaddleAmount(tc.in, tc.currency)
		if !errors.Is(err, billing.ErrAmountOverflow) {
			t.Errorf("parsePaddleAmount(%q) err = %v, want ErrAmountOverflow", tc.in, err)
		}
	}
}

// TestPaddleMinorUnitsRoundTrip proves the outbound formatter inverts the
// parser: every minor-unit refund amount encodes to the major-unit decimal
// Paddle expects and parses back to itself.
func TestPaddleMinorUnitsRoundTrip(t *testing.T) {
	t.Parallel()

	for _, v := range []int64{0, 1, 500, 1000, 2500, 9999, 10000, math.MaxInt64, math.MinInt64} {
		enc := billing.FormatMinorUnits(v, "USD")

		got, err := parsePaddleAmount(enc, "USD")
		if err != nil {
			t.Fatalf("round-trip(%d) err = %v", v, err)
		}

		if got != v {
			t.Errorf("round-trip(%d) = %d via %q", v, got, enc)
		}
	}
}

// TestRefundOutboundMajorUnits proves refund symmetry in minor units and that
// the adjustment item amount leaves the adapter as a major-unit decimal:
// refunding 1000 minor units of a 25.00 total sends "10.00", not "1000".
func TestRefundOutboundMajorUnits(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var itemAmount string

	mux := http.NewServeMux()
	mux.HandleFunc("/transactions/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, txnPayload("txn_sym", "paid", "USD", "25.00", []map[string]any{
			{"id": "txnitm_1", "totals": map[string]any{"total": "25.00"}},
		}))
	})
	mux.HandleFunc("/adjustments", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Items []struct {
				Amount *string `json:"amount"`
			} `json:"items"`
		}
		if derr := json.NewDecoder(r.Body).Decode(&body); derr != nil {
			http.Error(w, derr.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		defer mu.Unlock()

		if len(body.Items) == 1 && body.Items[0].Amount != nil {
			itemAmount = *body.Items[0].Amount
		}

		writeJSON(t, w, map[string]any{"data": map[string]any{"id": "adj_1"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := openTest(t, srv, nil)
	ctx := t.Context()

	if err := p.Refund(ctx, "txn_sym", 1000, ""); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if itemAmount != "10.00" {
		t.Fatalf("adjustment amount = %q, want major-unit %q", itemAmount, "10.00")
	}

	var over payment.AmountMismatchError

	if err := p.Refund(ctx, "txn_sym", 2501, ""); !errors.As(err, &over) || over.Expected != 2500 || over.Actual != 2501 {
		t.Fatalf("expected mismatch 2500/2501, got %v", err)
	}
}
