package migrate

import "errors"

// ErrUnsupportedDialect is returned by Plan (and anything that calls it) when
// the target database's dialect is not one this migration engine can diff
// against live state. Today that is every dialect except postgres, sqlite,
// and mysql -- most notably the Oracle/SQL Server family, which gets neither
// DDL rendering here nor bootstrap support. See doc.go for the full policy
// statement.
var ErrUnsupportedDialect = errors.New("orm/migrate: dialect not supported for live schema diffing (postgres, sqlite, and mysql only)")
