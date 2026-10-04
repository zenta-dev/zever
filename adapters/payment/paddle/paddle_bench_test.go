package paddle

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/payment"
)

func benchSign(body []byte, secret string, ts time.Time) string {
	stamp := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp + ":" + string(body)))
	return "ts=" + stamp + ";h1=" + hex.EncodeToString(mac.Sum(nil))
}

func benchWriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func benchServer(b *testing.B) *httptest.Server {
	b.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/transactions", func(w http.ResponseWriter, _ *http.Request) {
		benchWriteJSON(w, txnPayload("txn_bench", "ready", "USD", "100.00", nil))
	})
	mux.HandleFunc("/transactions/", func(w http.ResponseWriter, _ *http.Request) {
		benchWriteJSON(w, txnPayload("txn_full", "paid", "USD", "2500", []map[string]any{
			{"id": "txnitm_1", "totals": map[string]any{"total": "2500"}},
		}))
	})
	mux.HandleFunc("/adjustments", func(w http.ResponseWriter, _ *http.Request) {
		benchWriteJSON(w, map[string]any{"data": map[string]any{"id": "adj_bench"}})
	})
	srv := httptest.NewServer(mux)
	b.Cleanup(srv.Close)
	return srv
}

func benchPayment(b *testing.B) payment.Payment {
	b.Helper()
	srv := benchServer(b)
	p, err := New(payment.Options{APIKey: "pdl_api_test", Endpoint: srv.URL, WebhookSecret: "whsec_test"}) //nolint:gosec // synthetic test credential
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	b.Cleanup(func() { _ = p.Close() })
	return p
}

// BenchmarkCreatePayment measures the create round-trip against an in-process
// Paddle stub.
func BenchmarkCreatePayment(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	req := payment.Request{Amount: 10000, Currency: "USD", Meta: map[string]string{"price_id": "pri_main"}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.CreatePayment(ctx, req); err != nil {
			b.Fatalf("CreatePayment() = %v", err)
		}
	}
}

// BenchmarkCreatePaymentParallel measures concurrent creates.
func BenchmarkCreatePaymentParallel(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	req := payment.Request{Amount: 10000, Currency: "USD", Meta: map[string]string{"price_id": "pri_main"}}
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

// BenchmarkGetPayment measures the transaction fetch round-trip.
func BenchmarkGetPayment(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.GetPayment(ctx, "txn_full"); err != nil {
			b.Fatalf("GetPayment() = %v", err)
		}
	}
}

// BenchmarkRefund measures the full-refund round-trip (fetch + adjustment).
func BenchmarkRefund(b *testing.B) {
	p := benchPayment(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Refund(ctx, "txn_full", 2500, ""); err != nil {
			b.Fatalf("Refund() = %v", err)
		}
	}
}

// BenchmarkWebhookEvent measures signature verification and payload decode.
func BenchmarkWebhookEvent(b *testing.B) {
	p, err := New(payment.Options{APIKey: "pdl_api_test", WebhookSecret: "whsec_test"}) //nolint:gosec // synthetic test credential
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	b.Cleanup(func() { _ = p.Close() })

	body := []byte(`{"event_id":"evt_1","event_type":"transaction.paid","occurred_at":"2026-08-30T12:00:00Z","data":{"id":"txn_wh1","status":"paid","currency_code":"USD","details":{"totals":{"total":"100.00"}}}}`)
	sig := benchSign(body, "whsec_test", time.Now())
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.WebhookEvent(ctx, body, sig); err != nil {
			b.Fatalf("WebhookEvent() = %v", err)
		}
	}
}

// BenchmarkParsePaddleAmount measures the decimal-to-minor-units hot path.
func BenchmarkParsePaddleAmount(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := parsePaddleAmount("100.00"); err != nil {
			b.Fatalf("parsePaddleAmount() = %v", err)
		}
	}
}
