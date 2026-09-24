// Command server runs the showcase shop HTTP+gRPC API.
//
// Scaffolded by `zever generate server`. It is deliberately thin: parse
// flags, build the container, register HTTP routes and gRPC services,
// serve both, shut down both. Everything with behaviour worth testing
// belongs in a package of its own.
//
// Showcase addition: hand-written /api/* routes mount alongside the
// generated shop module (no path overlap: /api/* vs schema paths).
package main

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/zenta-dev/zever/authz"
	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
	"github.com/zenta-dev/zever/examples/showcase/internal/api"
	"github.com/zenta-dev/zever/examples/showcase/internal/app"
	shopimpl "github.com/zenta-dev/zever/examples/showcase/internal/service/shop"
	"github.com/zenta-dev/zever/middleware"
)

const (
	shutdownGrace     = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	readyTimeout      = 3 * time.Second

	// gRPC hardening defaults: bound idle connections, detect dead peers,
	// and cap message size so one client can't exhaust server memory.
	DefaultGRPCMaxConnectionIdle = 5 * time.Minute
	grpcKeepaliveTime            = 2 * time.Minute
	grpcKeepaliveTimeout         = 20 * time.Second
	grpcMaxMessageBytes          = 4 << 20 // 4 MiB, grpc-go's own default made explicit.
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP address to listen on")
	grpcAddr := flag.String("grpc-addr", ":9090", "gRPC address to listen on")

	flag.Parse()

	if err := run(*addr, *grpcAddr); err != nil {
		// The log battery is not necessarily available this early, so the
		// last-resort failure path writes to stderr directly.
		_, _ = os.Stderr.WriteString("server: " + err.Error() + "\n")

		os.Exit(1)
	}
}

func run(addr, grpcAddr string) error {
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

	authInst, err := c.Auth()
	if err != nil {
		return err
	}

	permInst, err := c.Permission()
	if err != nil {
		return err
	}

	obs, err := c.Observability()
	if err != nil {
		return err
	}

	limiter, err := c.RateLimit()
	if err != nil {
		return err
	}

	// One interceptor, attached once below, must cover every service this
	// process registers on grpcServer -- so every generated module
	// package's own GRPCPolicies() is merged into one map here first.
	policies := map[string]authz.Policy{}

	for k, v := range genshop.GRPCPolicies() {
		policies[k] = v
	}

	grpcServer, err := c.GRPC(
		grpc.ChainUnaryInterceptor(
			middleware.RecoverUnaryServerInterceptor(logger),
			middleware.TracingUnaryServerInterceptor(obs),
			middleware.RateLimitUnaryServerInterceptor(limiter, middleware.PeerAddrKey),
			authz.UnaryServerInterceptor(authInst, permInst, policies),
		),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: DefaultGRPCMaxConnectionIdle,
			Time:              grpcKeepaliveTime,
			Timeout:           grpcKeepaliveTimeout,
		}),
		grpc.MaxRecvMsgSize(grpcMaxMessageBytes),
		grpc.MaxSendMsgSize(grpcMaxMessageBytes),
	)
	if err != nil {
		return err
	}

	r, err := c.Router()
	if err != nil {
		return err
	}

	r.Use(middleware.Recover(logger), middleware.RequestLogger(logger), middleware.Tracing(obs))

	r.Use(middleware.RateLimit(limiter, middleware.RemoteAddrKey))

	database, err := c.DB()
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

	hasher, err := c.Password()
	if err != nil {
		return err
	}

	r.Handle("GET", "/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Handle("GET", "/readyz", func(w http.ResponseWriter, req *http.Request) {
		pingCtx, cancel := context.WithTimeout(req.Context(), readyTimeout)
		defer cancel()

		if err := database.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		w.WriteHeader(http.StatusOK)
	})

	api.New(database, authInst, hasher, limiter, permInst, q, i18nInst, flags).Routes(r)

	svcImpl := shopimpl.NewShopServiceImpl(shopimpl.Deps{DB: database, Queue: q, Logger: logger})

	genshop.RegisterModule(r, grpcServer, authInst, permInst, genshop.ModuleImpls{
		ShopService: svcImpl,
	})

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errs := make(chan error, 2)

	go func() {
		logger.Info().Str("addr", addr).Msg("http server listening")

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err

			return
		}

		errs <- nil
	}()

	go func() {
		lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", grpcAddr)
		if err != nil {
			errs <- err

			return
		}

		logger.Info().Str("addr", grpcAddr).Msg("grpc server listening")

		if err := grpcServer.Serve(lis); err != nil {
			errs <- err

			return
		}

		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info().Msg("server shutting down")

		grpcServer.GracefulStop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()

		return httpServer.Shutdown(shutdownCtx)
	}
}
