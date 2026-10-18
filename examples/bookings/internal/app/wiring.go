// Package app wires this project's service instances.
//
// The Ref vars below name each wired instance; Providers builds them from
// the named adapter constructors; Modules mounts the project's serve
// modules. Add batteries with `zever add <battery>`: it only inserts Ref
// vars and Provider entries here and never rewrites your code. Tune an
// instance with container.WithOptions entries in Providers.
package app

import (
	"context"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/examples/bookings/internal/api"
	"github.com/zenta-dev/zever/serve"

	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	permissionrbac "github.com/zenta-dev/zever/adapters/permission/rbac"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
)

// DB names the default sqlite instance (data/bookings.db unless DB_PATH overrides).
var DB = db.DefaultRef()

// Auth names the default jwt auth instance.
var Auth = auth.DefaultRef()

// Billing names the default stub billing instance.
var Billing = billing.DefaultRef()

// Cache names the default memory cache instance.
var Cache = cache.DefaultRef()

// Crypto names the default local crypto instance.
var Crypto = crypto.DefaultRef()

// Flag names the default static flag instance.
var Flag = flag.DefaultRef()

// Geo names the default static geo instance.
var Geo = geo.DefaultRef()

// I18n names the default embed i18n instance.
var I18n = i18n.DefaultRef()

// Idempotency names the default memory idempotency instance.
var Idempotency = idempotency.DefaultRef()

// Lock names the default memory lock instance.
var Lock = lock.DefaultRef()

// Log names the default slog logger instance.
var Log = log.DefaultRef()

// Observability names the default stdout provider instance.
var Observability = observability.DefaultRef()

// Password names the default argon2 hasher instance.
var Password = password.DefaultRef()

// Payment names the default stub payment instance.
var Payment = payment.DefaultRef()

// Permission names the default rbac checker instance.
var Permission = permission.DefaultRef()

// Queue names the default memory queue instance.
var Queue = queue.DefaultRef()

// RateLimit names the default memory ratelimit instance.
var RateLimit = ratelimit.DefaultRef()

// Router names the default stdhttp router instance.
var Router = router.DefaultRef()

// Scheduler names the default embedded scheduler instance.
var Scheduler = scheduler.DefaultRef()

// Tenant names the default single tenant instance.
var Tenant = tenant.DefaultRef()

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
		container.Provide(Billing, func(_ billing.Options) (billing.Billing, error) {
			return billingstub.New(), nil
		}),
		container.Provide(Cache, cachememory.New),
		container.Provide(Crypto, cryptolocal.New),
		container.Provide(Flag, flagstatic.New),
		container.Provide(Geo, geostatic.New),
		container.Provide(I18n, i18nembed.New),
		container.Provide(Idempotency, idempotencymemory.New),
		container.Provide(Lock, lockmemory.New),
		container.Provide(Log, container.WrapNoErr(logslog.New)),
		container.Provide(Observability, observabilitystdout.New),
		container.Provide(Password, passwordargon2.New),
		container.Provide(Payment, paymentstub.New),
		container.Provide(Permission, permissionrbac.New),
		container.Provide(Queue, queuememory.New),
		container.Provide(RateLimit, ratelimitmemory.New),
		container.Provide(Router, routerstdhttp.New),
		container.Provide(Scheduler, schedulerembedded.New),
		container.Provide(Tenant, tenantsingle.New),
	}
}

// Modules returns the serve modules mounted at boot, after the shared gRPC
// server (when enabled) is built. Each module registers its routes on the
// resolved router via Mount.
func Modules() func(*container.Container) []serve.Module {
	return func(*container.Container) []serve.Module {
		return []serve.Module{
			&apiModule{},
		}
	}
}

// apiModule mounts the bookings HTTP API routes onto the resolved router,
// resolving db, auth, password, and the hand-owned extra tables from the
// container.
type apiModule struct{}

// Name identifies the module in serve boot errors and logs.
func (m *apiModule) Name() string {
	return "api"
}

// Mount registers the bookings API routes on r and ensures the hand-owned
// side tables exist.
func (m *apiModule) Mount(r router.Router, c *container.Container) error {
	database, err := c.DB()
	if err != nil {
		return err
	}

	if err = api.EnsureExtraTables(context.Background(), database); err != nil {
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

	idem, err := c.Idempotency()
	if err != nil {
		return err
	}

	locker, err := c.Lock()
	if err != nil {
		return err
	}

	pay, err := c.Payment()
	if err != nil {
		return err
	}

	bill, err := c.Billing()
	if err != nil {
		return err
	}

	q, err := c.Queue()
	if err != nil {
		return err
	}

	crypt, err := c.Crypto()
	if err != nil {
		return err
	}

	ten, err := c.Tenant()
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

	perm, err := c.Permission()
	if err != nil {
		return err
	}

	geoInst, err := c.Geo()
	if err != nil {
		return err
	}

	api.New(database, authInst, hasher, limiter, idem, locker, pay, bill, q, crypt, ten, i18nInst, flags, perm, geoInst).Routes(r)

	return nil
}
