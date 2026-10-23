package payment

import (
	"testing"
)

func BenchmarkOptionsValidate(b *testing.B) {
	o := Options{MaxWebhookBytes: DefaultMaxWebhookBytes}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := o.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

func BenchmarkLimitDecode(b *testing.B) {
	raw := []byte(`{"id":"p_1","status":"succeeded","amount":100,"currency":"usd"}`)
	var out Result
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := LimitDecode(raw, &out, DefaultMaxWebhookBytes); err != nil {
			b.Fatalf("LimitDecode err = %v", err)
		}
	}
}

func BenchmarkCreatePayment(b *testing.B) {
	p := &stubPayment{result: Result{ID: "p_1", Status: PaymentSucceeded, Amount: 100, Currency: "usd"}}
	req := Request{Amount: 100, Currency: "usd", Method: MethodCard}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.CreatePayment(ctx, req); err != nil {
			b.Fatalf("CreatePayment err = %v", err)
		}
	}
}
