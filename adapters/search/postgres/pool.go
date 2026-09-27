package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultMaxConns caps the search pool when the caller does not specify a
// limit. It matches pgxpool's default of 4. When search, vectorstore, and db
// share one Postgres DSN, keep the sum of per-adapter caps below the
// server's max_connections; use PoolConfig with a lower maxConns for each
// adapter so the combined pools cannot exhaust the server.
const DefaultMaxConns = 4

// PoolConfig parses dsn into a pgxpool.Config and applies the MaxConns cap.
// A non-positive maxConns selects DefaultMaxConns. The DSN is never logged;
// errors carry only the parse failure.
func PoolConfig(dsn string, maxConns int) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: config: %w", err)
	}

	if maxConns <= 0 {
		maxConns = DefaultMaxConns
	}

	cfg.MaxConns = int32(maxConns) //nolint:gosec // G115: pool size bounded by design

	return cfg, nil
}
