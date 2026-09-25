// Package providers provides shared connection-option fields and SDK
// client constructors for the billing and payment packages' Stripe and
// Paddle adapters, which independently wrapped the same two SDKs with
// nearly-identical Options (SecretKey/APIKey/Endpoint/Sandbox), a
// literally-duplicated DefaultHTTPTimeout constant, and byte-for-byte
// duplicated client-construction logic (see billing/stripe.New vs
// payment/stripe.New, and billing/paddle.New vs payment/paddle.New before
// this package existed). Mirrors the internal/s3opts pattern already
// established for storage/s3, storage/r2, and media/s3's shared S3
// credential/endpoint handling.
package providers

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/PaddleHQ/paddle-go-sdk/v5"
	"github.com/stripe/stripe-go/v82"

	"github.com/zenta-dev/zever/internal/httpclient"
)

// DefaultHTTPTimeout is the default HTTP timeout for provider calls.
const DefaultHTTPTimeout = 30 * time.Second

// Common holds the provider connection fields billing.Options and
// payment.Options both need and validate identically: SecretKey/APIKey
// name the same two provider credential shapes (Stripe uses SecretKey,
// Paddle uses APIKey), Endpoint overrides the provider's default base URL,
// and Sandbox selects the provider's sandbox/test environment. Both
// packages embed this instead of redeclaring the fields, so the JSON/TOML/
// YAML shape and validation stay identical without hand-copying either.
type Common struct {
	// SecretKey holds the provider secret key (Stripe). It is never logged.
	SecretKey string `json:"secret_key" toml:"secret_key" yaml:"secret_key"`
	// APIKey holds the provider API key (Paddle). It is never logged.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// Endpoint holds the optional custom provider endpoint URL.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// Sandbox selects the provider sandbox environment.
	Sandbox bool `json:"sandbox" toml:"sandbox" yaml:"sandbox"`
}

// ValidateEndpoint checks that endpoint, when non-empty, is an absolute URL
// with both a scheme and a host, returning one error per violation (never
// joined) so a caller can wrap each into its own typed error -- e.g.
// billing.Options.Validate/payment.Options.Validate each want their own
// *InvalidOptionsError per violation, not one *InvalidOptionsError whose
// Reason is a multi-line joined message. An empty endpoint is valid (the
// provider's own default applies). This is the more detailed of the two
// validation messages billing/payment independently wrote before sharing
// this helper (payment's per-field "must include scheme"/"must include
// host" messages, rather than one generic "must be a valid url" covering
// every failure shape).
func ValidateEndpoint(endpoint string) []error {
	if endpoint == "" {
		return nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return []error{errors.New("endpoint must be a valid url")}
	}

	var errs []error

	if u.Scheme == "" {
		errs = append(errs, errors.New("endpoint must include scheme"))
	}

	if u.Host == "" {
		errs = append(errs, errors.New("endpoint must include host"))
	}

	return errs
}

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

// PaddleEndpoint resolves the Paddle API base URL: endpoint when set,
// else paddle.SandboxBaseURL or paddle.ProductionBaseURL per sandbox. Both
// billing/paddle.New and payment/paddle.New call this instead of
// separately duplicating the same three-way selection.
func PaddleEndpoint(endpoint string, sandbox bool) string {
	if endpoint != "" {
		return endpoint
	}

	if sandbox {
		return paddle.SandboxBaseURL
	}

	return paddle.ProductionBaseURL
}
