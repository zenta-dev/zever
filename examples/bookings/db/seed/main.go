// Command seed populates the bookings database with development data.
//
// Hand-written to match the `zever generate seed` shape. Run it against a
// database that `zever db migrate` has already created the tables in. The
// container is wired here directly (config.Resolve + explicit adapter
// registration) so this track stays disjoint from the sibling server track
// that owns internal/app.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/examples/bookings/internal/service/seed"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
)

const (
	shutdownGrace = 10 * time.Second
	seedTimeout   = 60 * time.Second
)

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString("seed: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	env, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("seed: load env: %w", err)
	}
	cfg, err := config.Resolve(env)
	if err != nil {
		return fmt.Errorf("seed: load config: %w", err)
	}

	c := container.New(cfg,
		container.Provide(log.DefaultRef(), func(o log.Options) (log.Logger, error) {
			return logslog.New(o), nil
		}),
		container.Provide(db.DefaultRef(), dbsqlite.New),
	)

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = c.Close(closeCtx)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()

	logger, err := c.Log()
	if err != nil {
		return err
	}

	database, err := c.DB()
	if err != nil {
		return err
	}

	if err := database.Ping(ctx); err != nil {
		return err
	}

	if err := seed.Run(ctx, database); err != nil {
		return err
	}

	logger.Info().Msg("seed complete")
	return nil
}
