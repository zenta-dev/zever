package adapters

import (
	"github.com/zenta-dev/zever/billing"
	billingpaddle "github.com/zenta-dev/zever/billing/paddle"
	billingstripe "github.com/zenta-dev/zever/billing/stripe"
	"github.com/zenta-dev/zever/payment"
	paymentpaddle "github.com/zenta-dev/zever/payment/paddle"
	paymentstripe "github.com/zenta-dev/zever/payment/stripe"
)

// RegisterPayments registers the provider-backed billing and payment
// adapters the core container no longer imports: stripe and paddle on both
// facades (sharing internal/providers for client construction). The
// in-memory stubs stay wired by the container. Registration only fills
// factory maps; it performs no I/O. Duplicate registrations are ignored, so
// calling RegisterPayments more than once (or alongside RegisterAll) is
// safe.
func RegisterPayments() {
	_ = billing.Register(billing.Stripe, billingstripe.New)
	_ = billing.Register(billing.Paddle, billingpaddle.New)

	_ = payment.Register(payment.Stripe, paymentstripe.New)
	_ = payment.Register(payment.Paddle, paymentpaddle.New)
}
