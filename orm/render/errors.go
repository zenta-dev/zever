package render

import (
	"errors"

	"github.com/zenta-dev/zever/orm/dialect"
)

// ErrTooManyArgs is returned by InsertMany (and its returning-aware twins)
// when rows*columns would overflow the argument-slice capacity -- i.e. the
// input is far beyond any dialect's parameter budget. There is no
// max-batch constant to reuse in orm (dialect budgets differ: Postgres
// ~65535 params, SQLite ~999-32766 depending on build flags), so this is a
// minimal overflow sentinel, not a policy limit. Callers test with
// errors.Is.
var ErrTooManyArgs = errors.New("orm/render: too many arguments")

// ErrUnsupported is an alias of dialect.ErrUnsupportedByDialect, kept so
// callers can match render's capability-gate failures (e.g. an aggregate
// whose SQL function has no equivalent on the resolved dialect) without
// importing orm/dialect. Both names denote the same sentinel value, so
// errors.Is works in either direction. Callers test with errors.Is.
var ErrUnsupported = dialect.ErrUnsupportedByDialect
