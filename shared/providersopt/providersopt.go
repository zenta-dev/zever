// Package providersopt holds the stdlib-only subset of provider
// connection-option fields shared by the billing and payment facades.
//
// It exists so the billing/payment Options structs (embedded providersopt.Common)
// compile without the Stripe/Paddle SDKs: only the concrete stripe/paddle
// adapter modules and shared/providersclient (SDK client constructors) import
// the SDKs. See shared/providersclient for the heavy half.
package providersopt

import (
	"errors"
	"net/url"
	"time"
)

// DefaultHTTPTimeout is the default HTTP timeout for provider calls.
const DefaultHTTPTimeout = 30 * time.Second

// PaddleSandboxBaseURL is the Paddle API base URL for the sandbox
// environment. It mirrors paddle.SandboxBaseURL without importing the SDK.
const PaddleSandboxBaseURL = "https://sandbox-api.paddle.com"

// PaddleProductionBaseURL is the Paddle API base URL for production.
// It mirrors paddle.ProductionBaseURL without importing the SDK.
const PaddleProductionBaseURL = "https://api.paddle.com"

// Common holds the provider connection fields billing.Options and
// payment.Options both need and validate identically: SecretKey/APIKey
// name the same two provider credential shapes (Stripe uses SecretKey,
// Paddle uses APIKey), Endpoint overrides the provider's default base URL,
// and Sandbox selects the provider's sandbox/test environment.
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
// joined) so a caller can wrap each into its own typed error. An empty
// endpoint is valid (the provider's own default applies).
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

// PaddleEndpoint resolves the Paddle API base URL: endpoint when set,
// else PaddleSandboxBaseURL or PaddleProductionBaseURL per sandbox.
func PaddleEndpoint(endpoint string, sandbox bool) string {
	if endpoint != "" {
		return endpoint
	}

	if sandbox {
		return PaddleSandboxBaseURL
	}

	return PaddleProductionBaseURL
}
