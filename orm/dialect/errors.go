package dialect

import "errors"

// ErrEmptyName is returned by Register when name is empty.
var ErrEmptyName = errors.New("orm/dialect: Register requires a non-empty name")

// ErrNilFactory is returned by Register when factory is nil.
var ErrNilFactory = errors.New("orm/dialect: Register requires a non-nil factory")

// ErrDuplicate is returned by Register when name is already registered.
var ErrDuplicate = errors.New("orm/dialect: Register called twice for dialect")

// ErrUnknownDialect is returned by For when name resolves to no factory.
var ErrUnknownDialect = errors.New("orm/dialect: unknown or unsupported dialect")

// ErrUnsupportedByDialect is the typed error query methods return when the
// resolved dialect lacks a capability the query needs. Callers test with
// errors.Is. orm/render.ErrUnsupported is an alias of this sentinel, so
// capability-gated failures match from either package.
var ErrUnsupportedByDialect = errors.New("orm/dialect: unsupported by dialect")
