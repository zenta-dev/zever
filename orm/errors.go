package orm

import "errors"

// ErrEmptyIdent indicates an identifier is empty.
var ErrEmptyIdent = errors.New("orm: empty identifier")

// ErrEmptyAlias indicates an alias is empty.
var ErrEmptyAlias = errors.New("orm: empty alias")

// ErrMutuallyExclusive indicates mutually exclusive options are both set.
var ErrMutuallyExclusive = errors.New("orm: mutually exclusive options")

// ErrScanTypeMismatch indicates a scan type mismatch.
var ErrScanTypeMismatch = errors.New("orm: scan type mismatch")

// ErrInvalidCTEName indicates an invalid CTE name.
var ErrInvalidCTEName = errors.New("orm: invalid CTE name")

// ErrTruncated indicates truncated data.
var ErrTruncated = errors.New("orm: truncated")

// ErrLockingWithDistinct is returned when DISTINCT is combined with
// FOR UPDATE/FOR SHARE. Standard SQL (and Postgres and SQLite) forbid
// locking rows in a DISTINCT result, so orm fails closed rather than
// emitting invalid SQL.
var ErrLockingWithDistinct = errors.New("orm: DISTINCT cannot be combined with a row lock (FOR UPDATE/FOR SHARE)")

// ErrLockingRequiresLockMode is returned when NOWAIT or SKIP LOCKED is
// used without a preceding FOR UPDATE/FOR SHARE.
var ErrLockingRequiresLockMode = errors.New("orm: NOWAIT and SKIP LOCKED require FOR UPDATE or FOR SHARE")

// ErrLockingNotSelect is returned when a row lock is requested on
// Count/Exists, which render an aggregate and cannot hold a row lock.
var ErrLockingNotSelect = errors.New("orm: row locking is only supported on SELECT queries, not Count/Exists")

// ErrCTEClauseRequiresRecursive is returned when a SEARCH or CYCLE clause is
// requested on a plain (non-recursive) CTE. Both clauses are defined only for
// a WITH RECURSIVE definition, so With(...).SearchDepthFirst(...) /
// .Cycle(...) is a caller error, rejected with a typed error before the
// dialect capability is even consulted. Callers test with errors.Is.
var ErrCTEClauseRequiresRecursive = errors.New("orm: SEARCH/CYCLE require a recursive CTE (use WithRecursive)")

// ErrCTEClauseEmptyColumns is returned when a SEARCH or CYCLE clause is
// requested with no BY columns. Both clauses require at least one column to
// order the traversal / identify a repeated row, so an empty list would
// render invalid SQL; it is rejected with a typed error instead. Callers
// test with errors.Is.
var ErrCTEClauseEmptyColumns = errors.New("orm: SEARCH/CYCLE require at least one BY column")

// ErrProjectionEmpty is returned when a projected query is executed with no
// projections: an empty projection list would render `SELECT FROM ...`,
// which is invalid on every dialect, so it fails closed before rendering.
var ErrProjectionEmpty = errors.New("orm: projected query requires at least one projection")

// ErrRetryable marks an error as a transient transaction failure worth
// retrying the WHOLE transaction over. RetryTx treats it (directly or
// wrapped anywhere in the error chain) as retryable, as does IsRetryable;
// wrapping a driver/dialect error with it is how a caller retries failures
// this helper does not recognize on its own -- the dialect-agnostic escape
// hatch. It is a sentinel for errors.Is/errors.Is-based testing, so wrap
// it with %w, never build its text.
var ErrRetryable = errors.New("orm: retryable transaction error")
