// Command server runs the todo HTTP API.
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

	"github.com/zenta-dev/zever/examples/todo/internal/api"
	"github.com/zenta-dev/zever/examples/todo/internal/app"
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

	r.Handle("GET", "/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	api.New(database, authInst, hasher).Routes(r)

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
