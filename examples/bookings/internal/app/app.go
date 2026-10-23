// Package app builds the config and container shared by this project's
// binaries.
//
// Explicit Register calls register exactly the adapters selected in
// Config is env-only. JWT secret comes from the environment only: set
// AUTH_JWT_SECRET to at least 32 bytes. It is never stored in a file.
package app

import (
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// DefaultDBPath is the sqlite file used when nothing else supplies one. It is
// relative to the process working directory (examples/bookings).
const DefaultDBPath = "data/bookings.db"

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by .env/.env.local discovery and by environment
// variables, with a sqlite path filled in when nothing supplied one.
//
// The JWT secret (AUTH_JWT_SECRET) comes from the environment only and is
// never defaulted here. Config fails closed when the secret is missing or
// shorter than 32 bytes.
func Config() (*config.Config, error) {
	env, err := config.FromEnv()
	if err != nil {
		return nil, fmt.Errorf("app: load env: %w", err)
	}
	cfg, err := config.Resolve(env)
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
	}

	if cfg.DB.Options.Path == "" {
		dbc := cfg.DB
		dbc.Options.Path = DefaultDBPath
		cfg.DB = dbc
	}

	if len(cfg.Auth.Options.JWT.Secret) < 32 {
		return nil, errors.New("app: AUTH_JWT_SECRET missing or shorter than 32 bytes: set AUTH_JWT_SECRET to at least 32 bytes")
	}

	return cfg, nil
}

// New builds the container every binary in this project uses. Nothing is
// opened until a battery is first requested.
func New() (*container.Container, error) {
	cfg, err := Config()
	if err != nil {
		return nil, err
	}

	return container.New(cfg, Providers()...), nil
}

// Register wires every adapter selected via env into its battery
// registry. Registration is idempotent and performs no I/O. New calls it;
// entrypoints that build the container themselves (cmd/server applies runtime
// defaults to the config before container.New) call it directly.
func Register() {
}
