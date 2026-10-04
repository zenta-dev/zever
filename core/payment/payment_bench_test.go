package payment

import (
	"testing"
)

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Register(freshAdapter(), func(Options) (Payment, error) { return &stubPayment{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Payment, error) { return &stubPayment{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Payment, error) { return &stubPayment{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Fatalf("Open err = %v", err)
			}
		}
	})
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseAdapter("stripe")
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	o := Options{MaxWebhookBytes: DefaultMaxWebhookBytes}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
		if _, err := p.CreatePayment(ctx, req); err != nil {
			b.Fatalf("CreatePayment err = %v", err)
		}
	}
}
