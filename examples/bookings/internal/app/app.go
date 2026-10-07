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

	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	permissionrbac "github.com/zenta-dev/zever/adapters/permission/rbac"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	searchdb "github.com/zenta-dev/zever/adapters/search/db"
	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	vectorstoredb "github.com/zenta-dev/zever/adapters/vectorstore/db"
	webhookqueue "github.com/zenta-dev/zever/adapters/webhook/queue"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
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

	Register()

	return container.New(cfg), nil
}

// Register wires every adapter selected in zever.yaml into its battery
// registry. Registration is idempotent and performs no I/O. New calls it;
// entrypoints that build the container themselves (cmd/server applies runtime
// defaults to the config before container.New) call it directly.
func Register() {
	analyticslog.Register()
	authjwt.Register()
	billingstub.Register()
	cachememory.Register()
	cryptolocal.Register()
	dbsqlite.Register()
	documentlocal.Register()
	eventbusmemory.Register()
	flagstatic.Register()
	geostatic.Register()
	i18nembed.Register()
	idempotencymemory.Register()
	lockmemory.Register()
	logslog.Register()
	mailerlog.Register()
	medialocal.Register()
	notificationlog.Register()
	observabilitystdout.Register()
	passwordargon2.Register()
	paymentstub.Register()
	permissionrbac.Register()
	queuememory.Register()
	ratelimitmemory.Register()
	routerstdhttp.Register()
	schedulerembedded.Register()
	searchdb.Register()
	secretsenv.Register()
	sessionmemory.Register()
	storagelocal.Register()
	tenantsingle.Register()
	vectorstoredb.Register()
	webhookqueue.Register()
	workflowmemory.Register()
}
