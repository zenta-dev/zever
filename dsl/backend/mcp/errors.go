package mcp

import "errors"

// ErrInternalInvariant reports a resolver-invariant violation reached inside
// the mcp backend: unreachable via the public API with real inputs.
var ErrInternalInvariant = errors.New("mcp: internal invariant violated")
