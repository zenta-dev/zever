// Package app builds the config and container shared by this project's
// binaries.
//
// It layers zever's zero-infra defaults, overlaid by zever.yaml in the
// working directory and by environment variables, then injects demo-friendly
// defaults for the batteries that need explicit material (file paths, the
// RBAC rules, the embedded i18n catalogs). The container resolves every
// other battery, including lock and secrets, with no hand-rolled wiring.
package app

import (
	"errors"
	"fmt"

	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/examples/demoapp/locales"

	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
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
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
)

const (
	// DefaultDBPath is the sqlite file used when nothing else supplies one.
	// It is relative to the process working directory (examples/demoapp).
	DefaultDBPath = "data/app.db"
	// DefaultSearchDSN is the sqlite file backing the search index.
	DefaultSearchDSN = "data/search.db"
	// DefaultVectorDSN is the sqlite file backing the vector index.
	DefaultVectorDSN = "data/vectors.db"
	// DefaultFlagsPath is the static flag file.
	DefaultFlagsPath = "flags.json"
	// DefaultCitiesPath is the static geo fixture.
	DefaultCitiesPath = "data/cities.json"
	// DefaultStorageRoot is the local storage root.
	DefaultStorageRoot = "tmp/demo-storage"
	// DefaultMediaRoot is the local media root.
	DefaultMediaRoot = "tmp/demo-media"
	// VectorDims is the demo embedding dimension.
	VectorDims = 8
)

// DevJWTSecret is an obvious dev-only HMAC secret used when AUTH_JWT_SECRET
// is unset. It is deterministic and public: never use it in production.
// It exists so `zever doctor` and the demo run with zero setup; production
// must export AUTH_JWT_SECRET with at least 32 random bytes.
//
// This app intentionally defaults rather than fails closed, to keep the
// zero-setup demo path working. Anything bootstrapped off this example
// toward a real deployment should instead copy examples/bookings'
// internal/app/app.go Config, which fails closed when AUTH_JWT_SECRET is
// missing or shorter than 32 bytes.
const DevJWTSecret = "dev-only-insecure-secret-32bytes!!" //nolint:gosec

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by any zever.yaml in the working directory and by environment
// variables, with demo file paths and rules filled in when nothing supplied
// them. Runtime defaults live here because internal/app is hand-owned and
// never regenerated, so entrypoints stay pure CLI shape.
func Config() (*config.Config, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
	}

	if cfg.DB.Options.Path == "" {
		cfg.DB.Options.Path = DefaultDBPath
	}

	if cfg.Auth.Options.JWT.Secret == "" {
		cfg.Auth.Options.JWT.Secret = DevJWTSecret
	}

	if cfg.Flag.Options.Static.Path == "" {
		cfg.Flag.Options.Static.Path = DefaultFlagsPath
	}

	if len(cfg.Permission.Options.Rules) == 0 {
		cfg.Permission.Options.Rules = []permission.Rule{
			{Role: "admin", Action: "*"},
			{Role: "member", Action: "product.read"},
			{Role: "member", Action: "order.create"},
			{Role: "member", Action: "post.create"},
			{Role: "member", Action: "post.read"},
		}
	}

	cfg.I18n.Options.Embed = i18n.EmbedOptions{FS: locales.FS, Dir: ".", Fallback: "en"}

	if cfg.Storage.Options.Root == "" {
		cfg.Storage.Options.Root = DefaultStorageRoot
	}

	if cfg.Media.Options.Root == "" {
		cfg.Media.Options.Root = DefaultMediaRoot
	}

	if cfg.Search.Options.DSN == "" {
		cfg.Search.Options.DSN = DefaultSearchDSN
	}

	if cfg.VectorStore.Options.DSN == "" {
		cfg.VectorStore.Options.DSN = DefaultVectorDSN
	}
	if cfg.VectorStore.Options.Dimension == 0 {
		cfg.VectorStore.Options.Dimension = VectorDims
	}

	if cfg.Geo.Options.Path == "" {
		cfg.Geo.Options.Path = DefaultCitiesPath
	}

	if len(cfg.Auth.Options.JWT.Secret) < 32 {
		return nil, errors.New("app: jwt secret shorter than 32 bytes: set AUTH_JWT_SECRET to at least 32 bytes")
	}

	return cfg, nil
}

// New builds the container every binary in this project uses. Nothing is
// opened until a battery is first requested. It registers the nested
// adapter modules the core container no longer wires itself.
func New() (*container.Container, error) {
	cfg, err := Config()
	if err != nil {
		return nil, err
	}

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
	webhookhttp.Register()
	workflowmemory.Register()

	return container.New(cfg), nil
}
