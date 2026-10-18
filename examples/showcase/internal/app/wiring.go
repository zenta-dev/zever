// Package app wires this project's service instances.
//
// The Ref vars below name each wired instance; Providers builds them from
// the named adapter constructors; Modules mounts the project's serve
// modules. Add batteries with `zever add <battery>`: it only inserts Ref
// vars and Provider entries here and never rewrites your code. Tune an
// instance with container.WithOptions entries in Providers.
package app

import (
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
	"github.com/zenta-dev/zever/examples/showcase/internal/api"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/shop"
	"github.com/zenta-dev/zever/serve"

	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	permissionrbac "github.com/zenta-dev/zever/adapters/permission/rbac"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
)

// DB names the default sqlite instance (data/showcase.db unless DB_PATH overrides).
var DB = db.DefaultRef()

// Auth names the default jwt auth instance.
var Auth = auth.DefaultRef()

// Crypto names the default local crypto instance.
var Crypto = crypto.DefaultRef()

// Flag names the default static flag instance.
var Flag = flag.DefaultRef()

// Geo names the default static geo instance.
var Geo = geo.DefaultRef()

// I18n names the default embed i18n instance.
var I18n = i18n.DefaultRef()

// Log names the default slog logger instance.
var Log = log.DefaultRef()

// Observability names the default stdout provider instance.
var Observability = observability.DefaultRef()

// Password names the default argon2 hasher instance.
var Password = password.DefaultRef()

// Permission names the default rbac checker instance.
var Permission = permission.DefaultRef()

// Queue names the default memory queue instance.
var Queue = queue.DefaultRef()

// RateLimit names the default memory ratelimit instance.
var RateLimit = ratelimit.DefaultRef()

// Router names the default stdhttp router instance.
var Router = router.DefaultRef()

// Providers returns every wired service instance, in declaration order.
// The container validates the graph (duplicate refs, missing deps, cycles)
// and resolves each instance lazily on first use. Runtime defaults apply
// only when the environment leaves a field unset, so env always wins.
func Providers() []container.Provider {
	return []container.Provider{
		container.Provide(DB, func(o db.Options) (db.DB, error) {
			if o.Path == "" {
				o.Path = DefaultDBPath
			}

			return dbsqlite.New(o)
		}),
		container.Provide(Auth, authjwt.New),
		container.Provide(Crypto, cryptolocal.New),
		container.Provide(Flag, flagstatic.New),
		container.Provide(Geo, geostatic.New),
		container.Provide(I18n, i18nembed.New),
		container.Provide(Log, container.WrapNoErr(logslog.New)),
		container.Provide(Observability, observabilitystdout.New),
		container.Provide(Password, passwordargon2.New),
		container.Provide(Permission, permissionrbac.New),
		container.Provide(Queue, queuememory.New),
		container.Provide(RateLimit, ratelimitmemory.New),
		container.Provide(Router, routerstdhttp.New),
	}
}

// Modules returns the serve modules mounted at boot, after the shared gRPC
// server (when enabled) is built. Each module registers its routes on the
// resolved router via Mount.
func Modules() func(*container.Container) []serve.Module {
	return func(*container.Container) []serve.Module {
		return []serve.Module{
			&apiModule{},
			&shopModule{},
		}
	}
}

// apiModule mounts the showcase HTTP API routes onto the resolved router.
type apiModule struct{}

// Name identifies the module in serve boot errors and logs.
func (m *apiModule) Name() string {
	return "api"
}

// Mount registers the showcase API routes on r.
func (m *apiModule) Mount(r router.Router, c *container.Container) error {
	database, err := c.DB()
	if err != nil {
		return err
	}

	authInst, err := c.Auth()
	if err != nil {
		return err
	}

	hasher, err := c.Password()
	if err != nil {
		return err
	}

	limiter, err := c.RateLimit()
	if err != nil {
		return err
	}

	perm, err := c.Permission()
	if err != nil {
		return err
	}

	q, err := c.Queue()
	if err != nil {
		return err
	}

	i18nInst, err := c.I18n()
	if err != nil {
		return err
	}

	flags, err := c.Flag()
	if err != nil {
		return err
	}

	api.New(database, authInst, hasher, limiter, perm, q, i18nInst, flags).Routes(r)

	return nil
}

// shopModule mounts the generated shop gRPC service onto the shared gRPC
// server, resolving auth, permission and the grpc server from the
// container. It also implements GRPCModule so its policies are merged into
// the fail-closed interceptor.
type shopModule struct{}

// Name identifies the module in serve boot errors and logs.
func (m *shopModule) Name() string {
	return "shop"
}

// GRPCPolicies returns the shop module's full-method policy map.
func (m *shopModule) GRPCPolicies() map[string]authz.Policy {
	return genshop.GRPCPolicies()
}

// Mount registers the shop HTTP routes and gRPC services on r and the
// resolved gRPC server.
func (m *shopModule) Mount(r router.Router, c *container.Container) error {
	database, err := c.DB()
	if err != nil {
		return err
	}

	q, err := c.Queue()
	if err != nil {
		return err
	}

	logger, err := c.Log()
	if err != nil {
		return err
	}

	authInst, err := c.Auth()
	if err != nil {
		return err
	}

	permInst, err := c.Permission()
	if err != nil {
		return err
	}

	grpcServer, err := c.GRPC()
	if err != nil {
		return err
	}

	svcImpl := shop.NewShopServiceImpl(shop.Deps{DB: database, Queue: q, Logger: logger})

	genshop.RegisterModule(r, grpcServer, authInst, permInst, genshop.ModuleImpls{
		ShopService: svcImpl,
	})

	return nil
}

// ensure shopModule implements serve.GRPCModule.
var _ serve.GRPCModule = (*shopModule)(nil)
