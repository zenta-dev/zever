package openapi

import "errors"

// ErrInternalInvariant reports a resolver-invariant violation reached inside
// the openapi backend (an unexpected ir.Validation.Kind or a non-numeric
// @validate value): unreachable via the public API with real inputs.
var ErrInternalInvariant = errors.New("openapi: internal invariant violated")
