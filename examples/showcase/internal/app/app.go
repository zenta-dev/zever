// Package app builds the config and container shared by this project's
// binaries.
//
// Explicit Register calls register exactly the adapters selected in
// zever.yaml. JWT secret comes from the environment only: set
// AUTH_JWT_SECRET to at least 32 bytes. It is never stored in zever.yaml.
package app

import (
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/examples/showcase/internal/api"

	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	searchsqlite "github.com/zenta-dev/zever/adapters/search/sqlite"
	vectorsqlite "github.com/zenta-dev/zever/adapters/vectorstore/sqlite"
)

// DefaultDBPath is the sqlite file used when nothing else supplies one. It is
// relative to the process working directory (examples/showcase).
const DefaultDBPath = "data/showcase.db"

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by any zever.yaml in the working directory and by environment
// variables, with a sqlite path filled in when nothing supplied one.
//
// Runtime defaults live here because internal/app is hand-owned/never
// regenerated, so entrypoints stay pure CLI shape.
//
// The JWT secret (AUTH_JWT_SECRET) comes from the environment only and is
// never defaulted here. Config fails closed when the secret is missing or
// shorter than 32 bytes.
func Config() (*config.Config, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("[app] load config: %w", err)
	}

	if cfg.DB.Options.Path == "" {
		dbc := cfg.DB
		dbc.Options.Path = DefaultDBPath
		cfg.DB = dbc
	}

	cfg.I18n.Options.Embed = i18n.EmbedOptions{FS: api.LocalesFS, Dir: "locales", Fallback: "en"}

	if len(cfg.Permission.Options.Rules) == 0 {
		cfg.Permission.Options.Rules = []permission.Rule{
			{Role: "admin", Action: "product.delete"},
			{Role: "user", Action: "product.delete", OwnedOnly: true, OwnedAttr: "owner"},
		}
	}

	if len(cfg.Auth.Options.JWT.Secret) < 32 {
		return nil, errors.New("[app] AUTH_JWT_SECRET missing or shorter than 32 bytes: set AUTH_JWT_SECRET to at least 32 bytes")
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

	authjwt.Register()
	billingstub.Register()
	dbsqlite.Register()
	documentlocal.Register()
	medialocal.Register()
	passwordargon2.Register()
	schedulerembedded.Register()
	searchsqlite.Register()
	vectorsqlite.Register()

	return container.New(cfg), nil
}
