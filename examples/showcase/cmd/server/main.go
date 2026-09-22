// Command server runs the showcase shop HTTP API.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/examples/showcase/internal/api"
	"github.com/zenta-dev/zever/examples/showcase/internal/app"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/middleware"
	"github.com/zenta-dev/zever/permission"
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
	cfg, err := app.Config()
	if err != nil {
		return err
	}

	// Runtime defaults the yaml file does not carry: embedded i18n catalog
	// and the product.delete owner rule for rbac.
	cfg.I18n.Options.Embed = i18n.EmbedOptions{FS: api.LocalesFS, Dir: "locales", Fallback: "en"}
	if len(cfg.Permission.Options.Rules) == 0 {
		cfg.Permission.Options.Rules = []permission.Rule{
			{Role: "admin", Action: "product.delete"},
			{Role: "user", Action: "product.delete", OwnedOnly: true, OwnedAttr: "owner"},
		}
	}

	c := container.New(cfg)

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

	authInst, err := c.Auth()
	if err != nil {
		return err
	}
	hasher, err := c.Password()
	if err != nil {
		return err
	}
	limiter, err := c.Ratelimit()
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
	provider, err := c.Observability()
	if err != nil {
		return err
	}

	r, err := c.Router()
	if err != nil {
		return err
	}

	r.Handle("GET", "/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Handle("GET", "/readyz", func(w http.ResponseWriter, req *http.Request) {
		pctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := database.Ping(pctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	api.New(database, authInst, hasher, limiter, perm, q, i18nInst, flags).Routes(r)

	var handler http.Handler = r
	handler = middleware.RequestLogger(logger)(handler)
	handler = middleware.Tracing(provider)(handler)
	if cfg.Ratelimit.Options.Rate > 0 {
		handler = middleware.RateLimit(limiter, middleware.RemoteAddrKey)(handler)
	}
	handler = middleware.Recover(logger)(handler)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info().Str("addr", addr).Msg("http server listening")
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
