package stripe

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

// centsState is scriptable state for the minor-units Stripe fake.
type centsState struct {
	mu           sync.Mutex
	refundAmount int64
	createAmount int64
}

// centsServer spins up a fake Stripe v1 API that echoes amounts verbatim and
// records the amounts it receives, so tests can prove the adapter performs
// no major/minor scaling in either direction.
func centsServer(t *testing.T, st *centsState) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/payment_intents", func(w http.ResponseWriter, r *http.Request) {
		amount, _ := strconv.Atoi(r.FormValue("amount"))

		st.mu.Lock()
		st.createAmount = int64(amount)
		st.mu.Unlock()

		writeJSON(w, map[string]any{
			"id":       "pi_cents_1",
			"object":   "payment_intent",
			"amount":   amount,
			"currency": r.FormValue("currency"),
			"status":   "succeeded",
		})
	})
	mux.HandleFunc("/v1/refunds", func(w http.ResponseWriter, r *http.Request) {
		amount, _ := strconv.Atoi(r.FormValue("amount"))

		st.mu.Lock()
		st.refundAmount = int64(amount)
		st.mu.Unlock()

		writeJSON(w, map[string]any{
			"id":             "re_cents_1",
			"object":         "refund",
			"amount":         amount,
			"payment_intent": "pi_cents_1",
		})
	})
	mux.HandleFunc("/v1/payment_intents/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id":       strings.TrimPrefix(r.URL.Path, "/v1/payment_intents/"),
			"object":   "payment_intent",
			"amount":   1000,
			"currency": "usd",
			"status":   "succeeded",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func openCents(t *testing.T, srv *httptest.Server) payment.Payment {
	t.Helper()

	p, err := New(payment.Options{SecretKey: "sk_test_cents", WebhookSecret: "whsec_cents", Endpoint: srv.URL}) //nolint:gosec // synthetic test credential
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return p
}

// TestMinorUnitsPassthrough proves Stripe amounts travel unscaled: creating
// 1000 minor units sends amount=1000 on the wire and returns 1000.
func TestMinorUnitsPassthrough(t *testing.T) {
	t.Parallel()

	var st centsState

	p := openCents(t, centsServer(t, &st))

	res, err := p.CreatePayment(t.Context(), payment.Request{Amount: 1000, Currency: "usd"})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	if res.Amount != 1000 {
		t.Fatalf("CreatePayment amount = %d, want 1000 minor units", res.Amount)
	}

	st.mu.Lock()
	sent := st.createAmount
	st.mu.Unlock()

	if sent != 1000 {
		t.Fatalf("wire amount = %d, want 1000 (no scaling)", sent)
	}
}

// TestRefundSymmetry proves refunds use the same minor units as creates:
// refunding 1000 sends amount=1000, matching the created total.
func TestRefundSymmetry(t *testing.T) {
	t.Parallel()

	var st centsState

	p := openCents(t, centsServer(t, &st))
	ctx := t.Context()

	res, err := p.CreatePayment(ctx, payment.Request{Amount: 1000, Currency: "usd"})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	if err := p.Refund(ctx, res.ID, res.Amount, ""); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if st.refundAmount != res.Amount {
		t.Fatalf("refund amount = %d, want created total %d", st.refundAmount, res.Amount)
	}
}

// TestGetPaymentMinorUnits proves fetched totals arrive unscaled.
func TestGetPaymentMinorUnits(t *testing.T) {
	t.Parallel()

	var st centsState

	got, err := openCents(t, centsServer(t, &st)).GetPayment(t.Context(), "pi_cents_1")
	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}

	if got.Amount != 1000 {
		t.Fatalf("GetPayment amount = %d, want 1000 minor units", got.Amount)
	}
}
