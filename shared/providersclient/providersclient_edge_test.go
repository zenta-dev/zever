package providersclient

import "testing"

// TestNewStripeClient_emptyEndpoint verifies the default-endpoint path builds
// a client without panicking.
func TestNewStripeClient_emptyEndpoint(t *testing.T) {
	t.Parallel()

	if c := NewStripeClient("sk_test", "", 0); c == nil {
		t.Fatal("NewStripeClient(empty endpoint) = nil")
	}
}
