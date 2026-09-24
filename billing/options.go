package billing

import (
	"errors"

	"github.com/zenta-dev/zever/internal/providers"
)

// DefaultHTTPTimeout is the default HTTP timeout for provider calls.
// Alias for providers.DefaultHTTPTimeout, which payment.DefaultHTTPTimeout
// also aliases -- previously both packages independently declared their
// own identical constant.
const DefaultHTTPTimeout = providers.DefaultHTTPTimeout

// Options configures billing backend selection and limits. SecretKey/
// APIKey/Endpoint/Sandbox are shared with payment.Options via
// providers.Common, validated identically in both packages.
type Options struct {
	providers.Common
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	endpointErrs := providers.ValidateEndpoint(o.Endpoint)
	errs := make([]error, 0, len(endpointErrs))

	for _, e := range endpointErrs {
		errs = append(errs, &InvalidOptionsError{Reason: e.Error()})
	}

	return errors.Join(errs...)
}
