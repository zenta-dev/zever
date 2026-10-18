// Package app builds the config and container shared by this project's
// binaries.
//
// The Ref vars below name each wired instance; Providers builds them from
// the named adapter constructors. This list was picked at scaffold time
// (`zever generate server`'s floor (log, router) when there was no prior `zever new`,
// or the project's own picks once it has one) -- add a battery later with
// `zever add <battery>`: it only inserts Ref vars and Provider entries
// here and never rewrites user code. Tune an instance with
// container.WithOptions entries in Providers.
package app

import (
	"fmt"

	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/router"
)

// DefaultDBPath is the sqlite file used when nothing else supplies one. It is
// relative to the process working directory.
const DefaultDBPath = "data/app.db"

// Config returns the resolved configuration: zever's zero-infra defaults,
// overlaid by .env/.env.local discovery and <SERVICE>_* environment
// variables, with a sqlite path filled in when nothing supplied
// one.
func Config() (*config.Config, error) {
	env, err := config.FromEnv()
	if err != nil {
		return nil, fmt.Errorf("app: load env: %w", err)
	}

	cfg, err := config.Resolve(env)
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
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

	return container.New(cfg, Providers()...), nil
}

// Log names the default log instance (slog adapter).
var Log = log.DefaultRef()

// Router names the default router instance (stdhttp adapter).
var Router = router.DefaultRef()

// Providers returns every wired service instance, in declaration order.
// The container validates the graph (duplicate refs, missing deps,
// cycles) and resolves each instance lazily on first use.
func Providers() []container.Provider {
	return []container.Provider{
		container.Provide(Log, container.WrapNoErr(logslog.New)),
		container.Provide(Router, routerstdhttp.New),
	}
}
