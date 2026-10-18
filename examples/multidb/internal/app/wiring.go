// Package app wires this project's multi-database service instances.
//
// It demonstrates wiring three sqlite databases through typed Refs:
//   - DB:       the primary (default) database
//   - DBAnalytics: a named "analytics" instance (DB__ANALYTICS__* env)
//   - DBAudit:    a named "audit" instance (DB__AUDIT__* env)
//
// Add batteries with `zever add <battery>`: it only inserts Ref vars and
// Provider entries here and never rewrites your code.
package app

import (
	"net/http"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/serve"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
)

// DB names the primary (default) sqlite instance.
var DB = db.DefaultRef()

// DBAnalytics names the named "analytics" sqlite instance, addressable via
// DB__ANALYTICS__* environment variables.
var DBAnalytics = db.Ref("analytics")

// DBAudit names the named "audit" sqlite instance, addressable via
// DB__AUDIT__* environment variables.
var DBAudit = db.Ref("audit")

// Log names the default slog logger instance.
var Log = log.DefaultRef()

// Router names the default stdhttp router instance.
var Router = router.DefaultRef()

// Providers returns every wired service instance, in declaration order.
// The container validates the graph (duplicate refs, missing deps, cycles)
// and resolves each instance lazily on first use.
func Providers() []container.Provider {
	return []container.Provider{
		container.Provide(DB, func(o db.Options) (db.DB, error) {
			if o.Path == "" {
				o.Path = "data/multidb.db"
			}

			return dbsqlite.New(o)
		}),
		container.Provide(DBAnalytics, func(o db.Options) (db.DB, error) {
			if o.Path == "" {
				o.Path = "data/analytics.db"
			}

			return dbsqlite.New(o)
		}),
		container.Provide(DBAudit, func(o db.Options) (db.DB, error) {
			if o.Path == "" {
				o.Path = "data/audit.db"
			}

			return dbsqlite.New(o)
		}),
		container.Provide(Log, container.WrapNoErr(logslog.New)),
		container.Provide(Router, routerstdhttp.New),
	}
}

// Modules returns the serve modules mounted at boot.
func Modules() func(*container.Container) []serve.Module {
	return func(*container.Container) []serve.Module {
		return []serve.Module{
			&dbModule{},
		}
	}
}

// dbModule demonstrates resolving all three databases from the container.
type dbModule struct{}

// Name identifies the module in serve boot errors and logs.
func (m *dbModule) Name() string {
	return "multidb"
}

// Mount registers a healthz route that pings all three databases.
func (m *dbModule) Mount(r router.Router, c *container.Container) error {
	primary, err := c.DB()
	if err != nil {
		return err
	}

	analytics, err := container.Get(c, DBAnalytics)
	if err != nil {
		return err
	}

	audit, err := container.Get(c, DBAudit)
	if err != nil {
		return err
	}

	r.Handle("GET", "/healthz", func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		_ = primary.Ping(ctx)
		_ = analytics.Ping(ctx)
		_ = audit.Ping(ctx)
		w.WriteHeader(http.StatusOK)
	})

	return nil
}
