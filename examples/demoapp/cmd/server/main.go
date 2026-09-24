// Command server runs the demoapp HTTP API — every battery exercised via
// HTTP against zero-infra adapters.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/examples/demoapp/internal/api"
	"github.com/zenta-dev/zever/examples/demoapp/internal/app"
)

const (
	shutdownGrace     = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP address to listen on")
	flag.Parse()

	if err := run(*addr); err != nil {
		_, _ = os.Stderr.WriteString("server: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func run(addr string) error {
	c, err := app.New()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = c.Close(closeCtx)
	}()

	logger, err := c.Log()
	if err != nil {
		return err
	}

	database, err := c.DB()
	if err != nil {
		return err
	}
	if err = database.Ping(ctx); err != nil {
		return fmt.Errorf("server: ping db: %w", err)
	}

	authInst, err := c.Auth()
	if err != nil {
		return err
	}

	hasher, err := c.Password()
	if err != nil {
		return err
	}

	r, err := c.Router()
	if err != nil {
		return err
	}

	// Best-effort battery resolve: the server stays up when an optional
	// battery is missing (e.g. no AI credentials). Handlers answer 501 for
	// nil batteries. Lock and secrets resolve through the container like
	// every other battery.
	d := api.Deps{DB: database, Auth: authInst, Password: hasher, Log: logger}
	d.Cache, _ = c.Cache()
	d.Flag, _ = c.Flag()
	d.Permission, _ = c.Permission()
	d.RateLimit, _ = c.RateLimit()
	d.Lock, _ = c.Lock()
	d.Idempotency, _ = c.Idempotency()
	d.Session, _ = c.Session()
	d.Queue, _ = c.Queue()
	d.Job, _ = c.Job()
	d.EventBus, _ = c.EventBus()
	d.Search, _ = c.Search()
	d.VectorStore, _ = c.VectorStore()
	d.Storage, _ = c.Storage()
	d.Media, _ = c.Media()
	d.AI, _ = c.AI()
	d.Geo, _ = c.Geo()
	d.I18n, _ = c.I18n()
	d.Crypto, _ = c.Crypto()
	d.Secrets, _ = c.Secrets()
	d.Notification, _ = c.Notification()
	d.Mailer, _ = c.Mailer()
	d.Webhook, _ = c.Webhook()
	d.Workflow, _ = c.Workflow()
	d.Observability, _ = c.Observability()
	d.Analytics, _ = c.Analytics()
	d.Payment, _ = c.Payment()
	d.Billing, _ = c.Billing()
	d.Document, _ = c.Document()
	d.Tenant, _ = c.Tenant()

	api.RegisterDemoWorkflow(d.Workflow)

	r.Handle("GET", "/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	api.New(d).Routes(r)

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info().Str("addr", addr).Msg("demo server listening")
		if serveErr := srv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errs <- serveErr
			return
		}
		errs <- nil
	}()

	select {
	case serveErr := <-errs:
		return serveErr
	case <-ctx.Done():
		logger.Info().Msg("server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
