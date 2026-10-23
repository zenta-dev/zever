// Package app builds the config and container shared by this project's
// binaries.
//
// Wiring lives in wiring.go: one container.Provide entry per battery naming
// the compiled-in adapter constructor. Changing an adapter means editing
// that entry and rebuilding; nothing is selected from the environment.
//
// JWT secret comes from the environment only: set AUTH_JWT_SECRET to at
// least 32 bytes. It is never stored in a file. `zever doctor` reports
// auth FAIL until the secret is set, which is expected by default.
package app

import (
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// DefaultDBPath is the sqlite file used when nothing else supplies one. It is
// relative to the process working directory.
const DefaultDBPath = "data/app.db"

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by .env/.env.local discovery and by environment
// variables, with a sqlite path filled in when nothing supplied one.
//
// The JWT secret (AUTH_JWT_SECRET) comes from the environment only and is
// never defaulted here; auth/jwt requires at least 32 bytes.
func Config() (*config.Config, error) {
	env, err := config.FromEnv()
	if err != nil {
		return nil, fmt.Errorf("app: load env: %w", err)
	}
	cfg, err := config.Resolve(env)
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
	}

	dbc := cfg.DB
	if dbc.Options.Path == "" {
		dbc.Options.Path = DefaultDBPath
	}

	cfg.DB = dbc

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
