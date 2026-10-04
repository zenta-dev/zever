package orm

import "errors"

// ErrEmptyIdent indicates an identifier is empty.
var ErrEmptyIdent = errors.New("empty identifier")

// ErrEmptyAlias indicates an alias is empty.
var ErrEmptyAlias = errors.New("empty alias")

// ErrMutuallyExclusive indicates mutually exclusive options are both set.
var ErrMutuallyExclusive = errors.New("mutually exclusive options")

// ErrScanTypeMismatch indicates a scan type mismatch.
var ErrScanTypeMismatch = errors.New("scan type mismatch")

// ErrInvalidCTEName indicates an invalid CTE name.
var ErrInvalidCTEName = errors.New("invalid CTE name")

// ErrTruncated indicates truncated data.
var ErrTruncated = errors.New("orm: truncated")
