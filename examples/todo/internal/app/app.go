// Package app builds the config and container shared by this project's
// binaries.
//
// The blank imports below are the whole of the "wiring" a zever app has to
// do: a battery's registry only learns about an adapter once that adapter
// package's init() has run, so an app imports exactly the adapters it
// actually uses and nothing else.
//
// JWT secret comes from the environment only: set AUTH_JWT_SECRET to at
// least 32 bytes. It is never stored in zever.yaml. `zever doctor` reports
// auth FAIL until the secret is set, which is expected by default.
package app

import (
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"

	// Blank imports register the adapters this app selects; see the package
	// comment.
	_ "github.com/zenta-dev/zever/auth/jwt"
	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/password/argon2"
	_ "github.com/zenta-dev/zever/queue/memory"
	_ "github.com/zenta-dev/zever/router/stdhttp"
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

	return container.New(cfg), nil
}
