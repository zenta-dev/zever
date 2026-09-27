package billing

import (
	"errors"

	"github.com/zenta-dev/zever/shared/providersopt"
)

// DefaultHTTPTimeout is the default HTTP timeout for provider calls.
// Alias for providersopt.DefaultHTTPTimeout, which payment.DefaultHTTPTimeout
// also aliases -- previously both packages independently declared their
// own identical constant.
const DefaultHTTPTimeout = providersopt.DefaultHTTPTimeout

// Options configures billing backend selection and limits. SecretKey/
// APIKey/Endpoint/Sandbox are shared with payment.Options via
// providersopt.Common, validated identically in both packages.
type Options struct {
	providersopt.Common
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	endpointErrs := providersopt.ValidateEndpoint(o.Endpoint)
	errs := make([]error, 0, len(endpointErrs))

	for _, e := range endpointErrs {
		errs = append(errs, &InvalidOptionsError{Reason: e.Error()})
	}

	return errors.Join(errs...)
}
