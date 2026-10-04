package providersclient

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/shared/httpclient"
)

// BenchmarkNewStripeClientWithHTTPClient measures constructing a Stripe client
// from a caller-supplied HTTP client.
func BenchmarkNewStripeClientWithHTTPClient(b *testing.B) {
	hc := httpclient.NewClient(time.Second)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if c := NewStripeClientWithHTTPClient("sk_test", "", hc); c == nil {
			b.Fatal("NewStripeClientWithHTTPClient() = nil")
		}
	}
}
