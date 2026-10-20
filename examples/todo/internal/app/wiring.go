// Package app wires this project's service instances.
//
// The Ref vars below name each wired instance; Providers builds them from
// the named adapter constructors; Modules mounts the project's serve
// modules. Add batteries with `zever add <battery>`: it only inserts Ref
// vars and Provider entries here and never rewrites your code. Tune an
// instance with container.WithOptions entries in Providers.
package app

import (
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/examples/todo/internal/api"
	"github.com/zenta-dev/zever/serve"
)

// DB names the default sqlite instance (data/app.db unless DB_PATH overrides).
var DB = db.DefaultRef()

// Cache names the default memory cache instance.
var Cache = cache.DefaultRef()

// Log names the default slog logger instance.
var Log = log.DefaultRef()

// Observability names the default stdout provider instance.
var Observability = observability.DefaultRef()

// Router names the default stdhttp router instance.
var Router = router.DefaultRef()

// Auth names the default jwt auth instance.
var Auth = auth.DefaultRef()

// Password names the default argon2 hasher instance.
var Password = password.DefaultRef()

// Queue names the default memory queue instance.
var Queue = queue.DefaultRef()

// Scheduler names the default embedded scheduler instance.
var Scheduler = scheduler.DefaultRef()

// RateLimit names the default ratelimit instance (memory adapter).
var RateLimit = ratelimit.DefaultRef()

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
		container.Provide(Cache, cachememory.New),
		container.Provide(Log, container.WrapNoErr(logslog.New)),
		container.Provide(Observability, observabilitystdout.New),
		container.Provide(Router, routerstdhttp.New),
		container.Provide(Auth, authjwt.New),
		container.Provide(Password, passwordargon2.New),
		container.Provide(Queue, queuememory.New),
		container.Provide(Scheduler, schedulerembedded.New),
		container.Provide(RateLimit, ratelimitmemory.New),
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

// apiModule mounts the todo HTTP API routes onto the resolved router,
// resolving db, auth and password from the container.
type apiModule struct{}

// Name identifies the module in serve boot errors and logs.
func (m *apiModule) Name() string {
	return "api"
}

// Mount registers the todo API routes on r.
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

	api.New(database, authInst, hasher).Routes(r)

	return nil
}
