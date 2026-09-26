// Package providersclient provides SDK client constructors for the billing
// and payment packages' Stripe adapters.
//
// The stdlib-only subset (Common, DefaultHTTPTimeout, ValidateEndpoint,
// PaddleEndpoint) lives in shared/providersopt so the billing/payment
// facades compile without the SDKs; this package keeps only the
// constructors that return SDK types.
package providersclient

import (
	"net/http"
	"time"

	"github.com/stripe/stripe-go/v82"

	"github.com/zenta-dev/zever/shared/httpclient"
)

// NewStripeClient builds a *stripe.Client against secretKey, using the
// shared httpclient.NewClient(timeout) transport and overriding the API
// base URL when endpoint is non-empty. Both billing/stripe.New and
// payment/stripe.New call this instead of separately constructing the
// same *stripe.BackendConfig/*stripe.Client.
func NewStripeClient(secretKey, endpoint string, timeout time.Duration) *stripe.Client {
	return NewStripeClientWithHTTPClient(secretKey, endpoint, httpclient.NewClient(timeout))
}

// NewStripeClientWithHTTPClient is NewStripeClient with a caller-supplied
// *http.Client, for payment/stripe (which also keeps a reference to the
// same client for webhook body reads) to avoid constructing two clients.
func NewStripeClientWithHTTPClient(secretKey, endpoint string, httpClient *http.Client) *stripe.Client {
	cfg := &stripe.BackendConfig{HTTPClient: httpClient}
	if endpoint != "" {
		cfg.URL = stripe.String(endpoint)
	}

	return stripe.NewClient(secretKey, stripe.WithBackends(stripe.NewBackendsWithConfig(cfg)))
}
