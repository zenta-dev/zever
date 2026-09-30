package postgres

import (
	"errors"
	"fmt"
	"time"
)

// DefaultTable is the workflow-runs table created when Options.Table is empty.
const DefaultTable = "workflow_runs"

// DefaultLeaseTTL is the claim lease granted to a run owner when
// Options.LeaseTTL is unset.
const DefaultLeaseTTL = 30 * time.Second

// DefaultConnectTimeout bounds pool connect during construction.
const DefaultConnectTimeout = 5 * time.Second

// DefaultOperationTimeout bounds a single DB round trip.
const DefaultOperationTimeout = 5 * time.Second

// Options holds typed configuration for the postgres workflow adapter.
// An empty DSN selects sqlite (Path, default ":memory:"); a set DSN opens
// postgres. Table defaults to DefaultTable; Owner identifies this replica's
// lease holder and must differ per replica that may reclaim work.
type Options struct {
	// DSN is the postgres connection string. Empty selects sqlite.
	DSN string `json:"dsn" toml:"dsn" yaml:"dsn"`
	// Path is the sqlite database file path. Default ":memory:".
	Path string `json:"path" toml:"path" yaml:"path"`
	// Table is the workflow-runs table name. Default DefaultTable.
	Table string `json:"table" toml:"table" yaml:"table"`
	// Owner identifies this replica as a lease holder.
	Owner string `json:"owner" toml:"owner" yaml:"owner"`
	// LeaseTTL is the claim lease granted to a run owner.
	LeaseTTL time.Duration `json:"lease_ttl" toml:"lease_ttl" yaml:"lease_ttl"`
	// MaxConns caps the connection pool size.
	MaxConns int `json:"max_conns" toml:"max_conns" yaml:"max_conns"`
	// MinConns is the minimum pool size (postgres) / idle target (sqlite).
	MinConns int `json:"min_conns" toml:"min_conns" yaml:"min_conns"`
	// MaxConnLifetime bounds how long a pooled connection may be reused.
	MaxConnLifetime time.Duration `json:"max_conn_lifetime" toml:"max_conn_lifetime" yaml:"max_conn_lifetime"`
	// MaxConnIdleTime bounds how long a pooled connection may stay idle.
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time" toml:"max_conn_idle_time" yaml:"max_conn_idle_time"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.MaxConns < 0 {
		errs = append(errs, errors.New("postgres: max_conns must not be negative"))
	}

	if o.MinConns < 0 {
		errs = append(errs, errors.New("postgres: min_conns must not be negative"))
	}

	if o.MaxConnLifetime < 0 {
		errs = append(errs, errors.New("postgres: max_conn_lifetime must not be negative"))
	}

	if o.MaxConnIdleTime < 0 {
		errs = append(errs, errors.New("postgres: max_conn_idle_time must not be negative"))
	}

	if o.LeaseTTL < 0 {
		errs = append(errs, errors.New("postgres: lease_ttl must not be negative"))
	}

	if o.Table != "" {
		if err := validateTableName(o.Table); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// validateTableName rejects table names outside [A-Za-z_][A-Za-z0-9_]*
// because the name is interpolated into DDL (orm has no DDL builder).
func validateTableName(name string) error {
	if name == "" {
		return errors.New("postgres: table must not be empty")
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')

		if !ok {
			return fmt.Errorf("postgres: invalid table name %q", name)
		}
	}

	return nil
}
