package providersclient

import (
	"testing"
)

func TestNewStripeClient_endpointOverride(t *testing.T) {
	t.Parallel()

	c := NewStripeClient("sk_test", "https://example.com", 0)
	if c == nil {
		t.Fatal("NewStripeClient returned nil")
	}
}
