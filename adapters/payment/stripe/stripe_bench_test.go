package stripe

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/zenta-dev/zever/core/payment"
)

func benchServer(b *testing.B) *httptest.Server {
	b.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/payment_intents", func(w http.ResponseWriter, r *http.Request) {
		amount, _ := strconv.Atoi(r.FormValue("amount"))
		writeJSON(w, map[string]any{
			"id":       "pi_bench",
			"object":   "payment_intent",
			"amount":   amount,
			"currency": r.FormValue("currency"),
			"status":   "succeeded",
		})
	})
	mux.HandleFunc("/v1/payment_intents/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"id": "pi_bench", "object": "payment_intent", "amount": 2000, "currency": "usd", "status": "succeeded",
		})
	})
	mux.HandleFunc("/v1/refunds", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"id": "re_bench", "object": "refund", "amount": 500, "payment_intent": "pi_bench"})
	})
	srv := httptest.NewServer(mux)
	b.Cleanup(srv.Close)
	return srv
}

func benchPayment(b *testing.B) payment.Payment {
	b.Helper()
	srv := benchServer(b)
	p, err := New(payment.Options{SecretKey: "sk_test_x", WebhookSecret: "whsec_test", Endpoint: srv.URL})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	b.Cleanup(func() { _ = p.Close() })
	return p
}

// BenchmarkCreatePayment measures the create round-trip against an in-process
// Stripe stub.
func BenchmarkCreatePayment(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	req := payment.Request{Amount: 2000, Currency: "usd"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.CreatePayment(ctx, req); err != nil {
			b.Fatalf("CreatePayment() = %v", err)
		}
	}
}

// BenchmarkCreatePaymentParallel measures concurrent creates through one
// driver.
func BenchmarkCreatePaymentParallel(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	req := payment.Request{Amount: 2000, Currency: "usd"}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := p.CreatePayment(ctx, req); err != nil {
				b.Errorf("CreatePayment() = %v", err)
				return
			}
		}
	})
}

// BenchmarkGetPayment measures the payment fetch round-trip.
func BenchmarkGetPayment(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.GetPayment(ctx, "pi_bench"); err != nil {
			b.Fatalf("GetPayment() = %v", err)
		}
	}
}

// BenchmarkRefund measures the refund round-trip without an idempotency store.
func BenchmarkRefund(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Refund(ctx, "pi_bench", 500, ""); err != nil {
			b.Fatalf("Refund() = %v", err)
		}
	}
}

// BenchmarkWebhookEvent measures signature verification and payload decode.
func BenchmarkWebhookEvent(b *testing.B) {
	p, err := New(payment.Options{SecretKey: "sk_test_x", WebhookSecret: "whsec_test"})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	b.Cleanup(func() { _ = p.Close() })

	raw := eventPayload("payment_intent.succeeded", piObject("pi_123", "succeeded"))
	sig := sign(raw, "whsec_test")
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.WebhookEvent(ctx, raw, sig); err != nil {
			b.Fatalf("WebhookEvent() = %v", err)
		}
	}
}

// BenchmarkRefundFingerprint measures the SHA-256 fingerprint hot path.
func BenchmarkRefundFingerprint(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := refundFingerprint("pi_bench", 500); len(got) == 0 {
			b.Fatal("refundFingerprint() empty")
		}
	}
}
