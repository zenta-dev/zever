package payment_test

import (
	"context"

	"github.com/zenta-dev/zever/payment"
	"github.com/zenta-dev/zever/payment/stub"
)

// ExampleOpen opens the stub backend and creates a payment.
func ExampleOpen() {
	_ = payment.Register(payment.Stub, stub.Open)

	p, err := payment.Open(payment.Stub, payment.Options{AutoApprove: true})
	if err != nil {
		return
	}
	defer p.Close()

	_, _ = p.CreatePayment(context.Background(), payment.Request{
		Amount:   100,
		Currency: "USD",
		Method:   payment.MethodCard,
	})
}
