package workflow

import (
	"errors"
	"fmt"
	"net"
)

// PostgresOptions holds connection settings for the postgres workflow
// adapter. It follows the core/queue RedisOptions precedent: adapter-specific
// connection configuration embedded in the shared Options so config,
// container, and Open flow through unchanged.
type PostgresOptions struct {
	// DSN is the postgres connection string. Empty selects sqlite
	// (dev/test); set DSN opens postgres for durable runs.
	DSN string `json:"dsn" toml:"dsn" yaml:"dsn"`
	// Table is the workflow-runs table name. Empty selects the adapter
	// default (DefaultTable in adapters/workflow/db).
	Table string `json:"table" toml:"table" yaml:"table"`
	// DedicatedPool opts out of container-level pool sharing. Default false
	// shares one pool per exact DSN; true opens a private pool.
	DedicatedPool bool `json:"dedicated_pool" toml:"dedicated_pool" yaml:"dedicated_pool"`
}

// Options holds typed configuration for workflow adapters.
type Options struct {
	// HostPort is the workflow server address.
	HostPort string `json:"host_port" toml:"host_port" yaml:"host_port"`
	// Namespace is the workflow namespace.
	Namespace string `json:"namespace" toml:"namespace" yaml:"namespace"`

	// PostgresOptions holds postgres-specific connection configuration.
	PostgresOptions
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.HostPort != "" {
		if _, _, err := net.SplitHostPort(o.HostPort); err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: fmt.Sprintf("invalid host_port %q", o.HostPort)})
		}
	}

	return errors.Join(errs...)
}
