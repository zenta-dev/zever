package paymenttest

import (
	"testing"
)

// BenchmarkCheckCreateGet measures the create/get assertion hot path.
func BenchmarkCheckCreateGet(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkCreateGet(ctx, healthyStubPayment()); err != nil {
			b.Fatalf("checkCreateGet() err = %v", err)
		}
	}
}

// BenchmarkCheckInvalidRequest measures the invalid-request assertion hot path.
func BenchmarkCheckInvalidRequest(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkInvalidRequest(ctx, healthyStubPayment()); err != nil {
			b.Fatalf("checkInvalidRequest() err = %v", err)
		}
	}
}

// BenchmarkCheckRefund measures the refund assertion hot path.
func BenchmarkCheckRefund(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkRefund(ctx, healthyStubPayment()); err != nil {
			b.Fatalf("checkRefund() err = %v", err)
		}
	}
}

// BenchmarkCheckRefundMismatch measures the refund-mismatch assertion hot path.
func BenchmarkCheckRefundMismatch(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkRefundMismatch(ctx, healthyStubPayment()); err != nil {
			b.Fatalf("checkRefundMismatch() err = %v", err)
		}
	}
}

// BenchmarkCheckWebhook measures the webhook assertion hot path.
func BenchmarkCheckWebhook(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkWebhook(ctx, healthyStubPayment()); err != nil {
			b.Fatalf("checkWebhook() err = %v", err)
		}
	}
}

// BenchmarkCheckClose measures the close assertion hot path.
func BenchmarkCheckClose(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkClose(healthyStubPayment()); err != nil {
			b.Fatalf("checkClose() err = %v", err)
		}
	}
}
