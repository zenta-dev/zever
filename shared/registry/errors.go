package registry

import "errors"

// ErrNilFactory is the default error returned by Register when the factory
// is nil or a typed nil and New was called with a nil errNil hook.
var ErrNilFactory = errors.New("registry: nil factory")

// ErrDuplicate is the sentinel wrapped by the default duplicate hook when
// Register is called with an adapter that is already registered.
var ErrDuplicate = errors.New("registry: duplicate adapter")

// ErrUnknown is the sentinel wrapped by the default unknown hook when Lookup
// is called with an adapter that was never registered.
var ErrUnknown = errors.New("registry: unknown adapter")
