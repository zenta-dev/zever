package shop_test

import (
	"fmt"
	"testing"

	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
)

// BenchmarkCheckoutMiss measures the nil-order fast path of Checkout.
func BenchmarkCheckoutMiss(b *testing.B) {
	svc := benchService(b)
	ctx := b.Context()
	req := &genshop.CheckoutRequest{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := svc.Checkout(ctx, req); err == nil {
			b.Fatalf("Checkout empty: want error")
		}
	}
}

// BenchmarkCreateProduct measures product insertion with unique names.
func BenchmarkCreateProduct(b *testing.B) {
	svc := benchService(b)
	ctx := b.Context()

	var n int
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		n++
		name := fmt.Sprintf("bench-%06d", n)
		if _, err := svc.CreateProduct(ctx, &genshop.CreateProductRequest{
			Name: name, PriceCents: 100,
		}); err != nil {
			b.Fatalf("CreateProduct: %v", err)
		}
	}
}
