// Command seed populates the demo database with development data.
//
// It loads config directly (bypassing app.New) so seeding needs no JWT
// secret. Run it from examples/demoapp against a database that
// `zever db migrate` has already created the tables in.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/demoapp/internal/service/seed"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
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
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("[seed] load config: %w", err)
	}
	documentlocal.Register()
	medialocal.Register()
	dbsqlite.Register()
	c := container.New(cfg)

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
