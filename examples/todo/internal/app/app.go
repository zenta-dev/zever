// Package app builds the config and container shared by this project's
// binaries.
//
// Explicit Register calls below are the whole of the "wiring" a zever app
// has to do: nested adapter modules expose no init magic, so an app calls
// Register for exactly the adapters it actually uses and nothing else.
//
// JWT secret comes from the environment only: set AUTH_JWT_SECRET to at
// least 32 bytes. It is never stored in zever.yaml. `zever doctor` reports
// auth FAIL until the secret is set, which is expected by default.
package app

import (
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"

	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
)

// DefaultDBPath is the sqlite file used when nothing else supplies one. It is
// relative to the process working directory.
const DefaultDBPath = "data/app.db"

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by any zever.yaml in the working directory and by environment
// variables, with a sqlite path filled in when nothing supplied one.
//
// The JWT secret (AUTH_JWT_SECRET) comes from the environment only and is
// never defaulted here; auth/jwt requires at least 32 bytes.
func Config() (*config.Config, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("[app] load config: %w", err)
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

	authjwt.Register()
	dbsqlite.Register()
	passwordargon2.Register()

	return container.New(cfg), nil
}
