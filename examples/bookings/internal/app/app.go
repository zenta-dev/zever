// Package app builds the config and container shared by this project's
// binaries.
//
// Blank imports register exactly the adapters selected in zever.yaml.
// JWT secret comes from the environment only: set AUTH_JWT_SECRET to at
// least 32 bytes. It is never stored in zever.yaml.
package app

import (
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"

	// Blank imports register the adapters this app selects.
	_ "github.com/zenta-dev/zever/analytics/log"
	_ "github.com/zenta-dev/zever/auth/jwt"
	_ "github.com/zenta-dev/zever/billing/stub"
	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/crypto/local"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/document/local"
	_ "github.com/zenta-dev/zever/eventbus/memory"
	_ "github.com/zenta-dev/zever/flag/static"
	_ "github.com/zenta-dev/zever/geo/static"
	_ "github.com/zenta-dev/zever/i18n/embed"
	_ "github.com/zenta-dev/zever/idempotency/memory"
	_ "github.com/zenta-dev/zever/lock/memory"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/mailer/log"
	_ "github.com/zenta-dev/zever/media/local"
	_ "github.com/zenta-dev/zever/notification/log"
	_ "github.com/zenta-dev/zever/observability/stdout"
	_ "github.com/zenta-dev/zever/password/argon2"
	_ "github.com/zenta-dev/zever/payment/stub"
	_ "github.com/zenta-dev/zever/permission/rbac"
	_ "github.com/zenta-dev/zever/queue/memory"
	_ "github.com/zenta-dev/zever/ratelimit/memory"
	_ "github.com/zenta-dev/zever/router/stdhttp"
	_ "github.com/zenta-dev/zever/scheduler/embedded"
	_ "github.com/zenta-dev/zever/search/sqlite"
	_ "github.com/zenta-dev/zever/secrets/env"
	_ "github.com/zenta-dev/zever/session/memory"
	_ "github.com/zenta-dev/zever/storage/local"
	_ "github.com/zenta-dev/zever/tenant/single"
	_ "github.com/zenta-dev/zever/vectorstore/sqlite"
	_ "github.com/zenta-dev/zever/webhook/queue"
	_ "github.com/zenta-dev/zever/workflow/memory"
)

// DefaultDBPath is the sqlite file used when nothing else supplies one. It is
// relative to the process working directory (examples/bookings).
const DefaultDBPath = "data/bookings.db"

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by any zever.yaml in the working directory and by environment
// variables, with a sqlite path filled in when nothing supplied one.
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

	return container.New(cfg), nil
}
