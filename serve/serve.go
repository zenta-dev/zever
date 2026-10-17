package serve

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/middleware"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
)

const (
	shutdownGrace     = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	readyTimeout      = 3 * time.Second

	grpcMaxConnectionIdle = 5 * time.Minute
	grpcKeepaliveTime     = 2 * time.Minute
	grpcKeepaliveTimeout  = 20 * time.Second
	grpcMaxMessageBytes   = 4 << 20 // 4 MiB, grpc-go's own default made explicit.
)

// DefaultMaxRequestBodyBytes bounds HTTP request bodies: one client cannot
// exhaust server memory. It matches the gRPC max message size.
const DefaultMaxRequestBodyBytes = 4 << 20 // 4 MiB.

// Module is one mountable unit of a generated server: a name plus the route
// registration that wires its handlers onto the resolved router. Mount runs
// once at boot, after the gRPC server (when enabled) is built: a module that
// serves gRPC calls c.GRPC() inside Mount to register its services on the
// shared instance.
//
// Modules that declare gRPC methods also implement GRPCModule so serve can
// merge their per-method policies into the single fail-closed interceptor
// before the server is built. What the generated RegisterModule wrapper
// needs (auth, permission, ModuleImpls) it resolves from c itself.
type Module interface {
	// Name identifies the module in errors and logs.
	Name() string
	// Mount registers the module's routes (and gRPC services) on r.
	Mount(r router.Router, c *container.Container) error
}

// GRPCModule is the optional face of a Module that serves gRPC: the
// per-method policies merged into the fail-closed interceptor. Methods
// without a policy entry are denied; the standard health check is
// allowlisted by serve itself. Public (auth: none) operations carry an
// explicit empty Policy.
type GRPCModule interface {
	Module
	// GRPCPolicies returns the module's full-method policy map.
	GRPCPolicies() map[string]authz.Policy
}

// Run boots a generated server to completion: it blocks until SIGINT or
// SIGTERM, then shuts down gracefully and returns. It never panics and
// never calls log.Fatal or os.Exit: the caller prints the returned error
// to stderr and exits non-zero.
func Run(providers []container.Provider, modules func(*container.Container) []Module, opts ...Option) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runServe(ctx, providers, modules, opts...)
}

// Main runs a generated server to completion: it blocks until SIGINT or
// SIGTERM, then shuts down gracefully and returns. It never panics and
// never calls log.Fatal: the caller prints the returned error to stderr
// and exits non-zero.
//
// Main is Run plus the `doctor` dispatch: when os.Args carries `doctor`
// or `--doctor`, Main prints the resolve-all JSON report to stdout and
// returns nil (or ErrDoctorFailed) instead of serving.
//
// Deviation from the v0.8.0 plan spec (loud, deliberate): the plan sketches
// Main as void, printing to stderr and calling os.Exit(1) itself. Main
// instead returns the error so the generated main owns the exit path and
// the boot stays unit-testable; no os.Exit lives in this library.
func Main(providers []container.Provider, modules func(*container.Container) []Module, opts ...Option) error {
	if isDoctorArgs(os.Args[1:]) {
		return runDoctorReport(os.Stdout, providers, opts...)
	}

	return Run(providers, modules, opts...)
}

// Worker runs a queue/scheduler worker to completion: it blocks until
// SIGINT or SIGTERM, then shuts down gracefully and returns. It never
// listens on HTTP; job registration lives in the generated worker main,
// serve only owns boot, scheduler start, wait, and close.
func Worker(providers []container.Provider, opts ...Option) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runWorker(ctx, providers, opts...)
}

// resolveEnvConfig loads the process (or override) env and resolves it to
// a validated Config. Every failure names the fix; secret values never echo.
func resolveEnvConfig(o options) (*config.Config, error) {
	env := o.env
	if !o.hasEnv {
		var err error

		env, err = config.FromEnv()
		if err != nil {
			return nil, fmt.Errorf("serve: load env: %w", err)
		}
	}

	cfg, err := config.Resolve(env)
	if err != nil {
		return nil, fmt.Errorf("serve: load config: %w", err)
	}

	return cfg, nil
}

// guardDevKey refuses the deterministic dev crypto key outside a dev
// ZEVER_ENV. It inspects config only, opening nothing, and only for the
// legacy path: provider-wired crypto owns its own key via WithOptions and
// is enforced by the crypto adapter at open time.
func guardDevKey(c *container.Container, cfg *config.Config) error {
	for _, wired := range c.Wired() {
		if wired == "crypto" {
			return nil
		}
	}

	if crypto.IsDevCryptoKey(cfg.Crypto.Options.Key) && !crypto.DevKeyAllowed(cfg.Crypto.Options) {
		return errors.New("serve: refusing deterministic dev crypto key in non-dev environment: set a real CRYPTO_KEY, or run with ZEVER_ENV=dev|test|development (or allow_dev_key: true)")
	}

	return nil
}

// collectModules calls modulesFn and rejects duplicate names fail-closed.
func collectModules(c *container.Container, modulesFn func(*container.Container) []Module) ([]Module, error) {
	if modulesFn == nil {
		return nil, nil
	}

	mods := modulesFn(c)

	seen := make(map[string]bool, len(mods))

	for _, mod := range mods {
		if mod == nil {
			continue
		}

		if seen[mod.Name()] {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateModule, mod.Name())
		}

		seen[mod.Name()] = true
	}

	return mods, nil
}

// mergeGRPCPolicies folds every GRPCModule's policy map into one, plus the
// allowlisted health check. Later modules win on conflict, matching the
// generated server template.
func mergeGRPCPolicies(mods []Module) map[string]authz.Policy {
	policies := make(map[string]authz.Policy)

	for _, mod := range mods {
		if gm, ok := mod.(GRPCModule); ok {
			for k, v := range gm.GRPCPolicies() {
				policies[k] = v
			}
		}
	}

	policies[healthpb.Health_Check_FullMethodName] = authz.Policy{}

	return policies
}

// requireGRPCAddr fails closed when modules declare gRPC policies but no
// gRPC address is configured. The health-only entry does not count.
func requireGRPCAddr(mods []Module, grpcAddr string) error {
	if grpcAddr != "" {
		return nil
	}

	for _, mod := range mods {
		if gm, ok := mod.(GRPCModule); ok && len(gm.GRPCPolicies()) > 0 {
			return fmt.Errorf("serve: module %q declares gRPC policies but no gRPC address is set: %w (set GRPC_ADDR=:9090 or WithGRPCAddr)", mod.Name(), ErrGRPCPoliciesWithoutAddr)
		}
	}

	return nil
}

// noSuchTableHint names the migrate command for missing-table failures.
// Full sentinel coverage is stream E's work; this catches the obvious
// sqlite/postgres message shape at the boundary.
const noSuchTableHint = "pending migrations? run `zever db migrate`"

// isNoSuchTable reports whether err looks like a missing-table failure.
func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// withMigrateHint wraps a resolve-db failure, appending the migrate command
// when err looks like a missing table. Other errors pass through with serve
// context only.
func withMigrateHint(err error) error {
	if err == nil {
		return nil
	}

	if isNoSuchTable(err) {
		return fmt.Errorf("serve: resolve db: %w (%s)", err, noSuchTableHint)
	}

	return fmt.Errorf("serve: resolve db: %w", err)
}

// isDoctorArgs reports whether the binary was asked for a doctor report,
// either as `app doctor` or `app --doctor` (the flag form the zever CLI
// execs against a built server).
func isDoctorArgs(args []string) bool {
	for _, arg := range args {
		if arg == "doctor" || arg == "--doctor" {
			return true
		}
	}

	return false
}

// lookupEnv reads one process variable. It is a seam so tests can rely on
// t.Setenv for ambient behavior.
func lookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

// isLoopbackAddr reports whether addr binds loopback only. Empty and
// wildcard hosts bind every interface, and an unparsable address fails
// closed to warning.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "", "0.0.0.0", "::":
		return false
	case "localhost":
		return true
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))

	return ip != nil && ip.IsLoopback()
}

// maxBodyHandler bounds request bodies: handlers reading past the limit see
// a read error instead of buffering unbounded input.
func maxBodyHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, DefaultMaxRequestBodyBytes)
		}

		next.ServeHTTP(w, r)
	})
}

// listen binds addr and reports it on the hook. EADDRINUSE fails with the
// exact variable to change, never raw.
func listen(ctx context.Context, network, addr, envVar string, onListen func(network, addr string)) (net.Listener, error) {
	lis, err := (&net.ListenConfig{}).Listen(ctx, network, addr)
	if err != nil {
		return nil, listenError(addr, envVar, err)
	}

	if onListen != nil {
		onListen(network, lis.Addr().String())
	}

	return lis, nil
}

// listenError maps a bind failure to an actionable error.
func listenError(addr, envVar string, err error) error {
	if errors.Is(err, syscall.EADDRINUSE) {
		if _, port, splitErr := net.SplitHostPort(addr); splitErr == nil {
			return fmt.Errorf("serve: listen on %s: port %s in use; set %s=%s (or stop the other process): %w", addr, port, envVar, nextPort(port), err)
		}
	}

	return fmt.Errorf("serve: listen on %s: %w", addr, err)
}

// nextPort suggests the adjacent port as a concrete next step.
func nextPort(port string) string {
	var n int

	if _, scanErr := fmt.Sscanf(port, "%d", &n); scanErr != nil || n <= 0 || n >= 65535 {
		return "<free-port>"
	}

	return strconv.Itoa(n + 1)
}

// runServe boots the container, mounts modules, and serves until ctx ends.
func runServe(ctx context.Context, providers []container.Provider, modulesFn func(*container.Container) []Module, opts ...Option) error {
	o := foldOptions(opts)

	cfg, err := resolveEnvConfig(o)
	if err != nil {
		return err
	}

	c := container.New(cfg, providers...)

	if err = guardDevKey(c, cfg); err != nil {
		return err
	}

	mods, err := collectModules(c, modulesFn)
	if err != nil {
		return err
	}

	grpcAddr := o.grpcAddrResolved()

	if err = requireGRPCAddr(mods, grpcAddr); err != nil {
		return err
	}

	logger, err := c.Log()
	if err != nil {
		return fmt.Errorf("serve: resolve log: %w", err)
	}

	obs, err := c.Observability()
	if err != nil {
		return fmt.Errorf("serve: resolve observability: %w", err)
	}

	r, err := c.Router()
	if err != nil {
		return fmt.Errorf("serve: resolve router: %w", err)
	}

	limiter, keyFunc, err := resolveRateLimit(c, cfg)
	if err != nil {
		return err
	}

	tlsCfg, err := loadTLS(cfg)
	if err != nil {
		return err
	}

	securityOpts := middleware.SecurityHeadersOptions{}
	if tlsCfg != nil {
		securityOpts.HSTSMaxAge = middleware.DefaultHSTSMaxAge
	}

	r.Use(middleware.Recover(logger), middleware.RequestLogger(logger), middleware.Tracing(obs), middleware.SecurityHeaders(securityOpts))
	if limiter != nil {
		r.Use(middleware.RateLimit(limiter, keyFunc))
	}

	if _, err = c.DB(); err != nil {
		return withMigrateHint(err)
	}

	grpcServer, healthServer, err := setupGRPC(c, mods, logger, obs, limiter, tlsCfg, grpcAddr)
	if err != nil {
		return err
	}

	for _, mod := range mods {
		if mod == nil {
			continue
		}

		if mountErr := mod.Mount(r, c); mountErr != nil {
			return fmt.Errorf("serve: mount module %q: %w", mod.Name(), mountErr)
		}
	}

	r.Handle("GET", "/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Handle("GET", "/readyz", func(w http.ResponseWriter, req *http.Request) {
		readyCtx, cancel := context.WithTimeout(req.Context(), readyTimeout)
		defer cancel()

		if readyErr := c.Ready(readyCtx); readyErr != nil {
			if healthServer != nil {
				healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
			}

			if isNoSuchTable(readyErr) {
				logger.Warn().Err(readyErr).Msg("readiness failed: " + noSuchTableHint)
			}

			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		if healthServer != nil {
			healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		}

		w.WriteHeader(http.StatusOK)
	})

	pprofAddr := o.pprofAddrResolved()

	//nolint:contextcheck // container.StartPprof takes no ctx; the listener binds before serving and stops via shutdownPprof with a derived timeout.
	pprofSrv, err := container.StartPprof(pprofAddr, logger)
	if err != nil {
		return fmt.Errorf("serve: start pprof: %w", err)
	}

	defer shutdownPprof(ctx, pprofSrv)

	httpAddr := net.JoinHostPort("", o.httpPort())
	if tlsCfg == nil {
		for _, a := range []string{httpAddr, grpcAddr} {
			if a != "" && !isLoopbackAddr(a) {
				logger.Warn().Str("addr", a).Msg("serving plaintext without TLS on a non-loopback address; set router tls_cert and tls_key")
			}
		}
	}

	httpLis, err := listen(ctx, "tcp", httpAddr, "PORT", o.onListen)
	if err != nil {
		return err
	}

	var grpcLis net.Listener

	if grpcAddr != "" {
		grpcLis, err = listen(ctx, "tcp", grpcAddr, "GRPC_ADDR", o.onListen)
		if err != nil {
			return err
		}
	}

	httpServer := &http.Server{
		Addr:              httpAddr,
		Handler:           maxBodyHandler(r),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		TLSConfig:         tlsCfg,
	}

	return serveUntilDone(ctx, c, httpServer, httpLis, tlsCfg != nil, grpcServer, grpcLis, healthServer, logger, grpcAddr, httpAddr)
}

// serveUntilDone runs the listeners until a server fails or ctx ends, then
// shuts down gracefully in dependency order.
func serveUntilDone(ctx context.Context, c *container.Container, httpServer *http.Server, httpLis net.Listener, useTLS bool, grpcServer *grpc.Server, grpcLis net.Listener, healthServer *health.Server, logger log.Logger, grpcAddr, httpAddr string) error {
	errs := make(chan error, 2)

	go func() {
		logger.Info().Str("addr", httpAddr).Msg("http server listening")

		var serveErr error
		if useTLS {
			serveErr = httpServer.ServeTLS(httpLis, "", "")
		} else {
			serveErr = httpServer.Serve(httpLis)
		}

		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errs <- serveErr

			return
		}

		errs <- nil
	}()

	if grpcServer != nil && grpcLis != nil {
		go func() {
			logger.Info().Str("addr", grpcAddr).Msg("grpc server listening")

			if serveErr := grpcServer.Serve(grpcLis); serveErr != nil {
				errs <- serveErr

				return
			}

			errs <- nil
		}()
	}

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info().Str("addr", httpAddr).Msg("server shutting down")

		if healthServer != nil {
			healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		}

		if grpcServer != nil {
			grpcServer.GracefulStop()
		}

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("serve: shutdown http: %w", err)
		}

		closeCtx, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancelClose()

		return c.Close(closeCtx)
	}
}

// shutdownPprof stops the pprof listener, bounded by shutdownGrace. ctx may
// already be canceled, so it is stripped before deriving the timeout.
func shutdownPprof(ctx context.Context, pprofSrv *container.PprofServer) {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()

	_ = pprofSrv.Shutdown(shutdownCtx)
}

// runWorker boots the container, starts the scheduler, and waits for ctx.
func runWorker(ctx context.Context, providers []container.Provider, opts ...Option) error {
	o := foldOptions(opts)

	cfg, err := resolveEnvConfig(o)
	if err != nil {
		return err
	}

	c := container.New(cfg, providers...)

	if err = guardDevKey(c, cfg); err != nil {
		return err
	}

	logger, err := c.Log()
	if err != nil {
		return fmt.Errorf("serve: resolve log: %w", err)
	}

	if _, err = c.Queue(); err != nil {
		return fmt.Errorf("serve: resolve queue: %w", err)
	}

	sched, err := c.Scheduler()
	if err != nil {
		return fmt.Errorf("serve: resolve scheduler: %w", err)
	}

	if err = sched.Start(); err != nil {
		return fmt.Errorf("serve: start scheduler: %w", err)
	}

	//nolint:contextcheck // container.StartPprof takes no ctx; the listener binds before serving and stops via shutdownPprof with a derived timeout.
	pprofSrv, err := container.StartPprof(o.pprofAddrResolved(), logger)
	if err != nil {
		return fmt.Errorf("serve: start pprof: %w", err)
	}

	defer shutdownPprof(ctx, pprofSrv)

	logger.Info().Msg("worker started")

	<-ctx.Done()

	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()

	return c.Close(closeCtx)
}

// setupGRPC builds the shared gRPC server with the fail-closed interceptor
// chain, or returns nils when grpcAddr is empty. Reflection follows
// GRPC_REFLECTION, defaulting to dev ZEVER_ENV environments.
func setupGRPC(c *container.Container, mods []Module, logger log.Logger, obs observability.Provider, limiter ratelimit.Limiter, tlsCfg *tls.Config, grpcAddr string) (*grpc.Server, *health.Server, error) {
	if grpcAddr == "" {
		return nil, nil, nil
	}

	authInst, err := c.Auth()
	if err != nil {
		return nil, nil, fmt.Errorf("serve: resolve auth: %w", err)
	}

	permInst, err := c.Permission()
	if err != nil {
		return nil, nil, fmt.Errorf("serve: resolve permission: %w", err)
	}

	// One interceptor, attached once below, covers every service registered
	// on the shared server: fail-closed (deny-by-default when no policy),
	// with the standard health check allowlisted so probes keep working.
	policies := mergeGRPCPolicies(mods)

	grpcOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			middleware.RecoverUnaryServerInterceptor(logger),
			middleware.TracingUnaryServerInterceptor(obs),
			rateLimitUnaryInterceptor(limiter),
			authz.UnaryServerInterceptorStrict(authInst, permInst, policies),
		),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: grpcMaxConnectionIdle,
			Time:              grpcKeepaliveTime,
			Timeout:           grpcKeepaliveTimeout,
		}),
		grpc.MaxRecvMsgSize(grpcMaxMessageBytes),
		grpc.MaxSendMsgSize(grpcMaxMessageBytes),
	}
	if tlsCfg != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	grpcServer, err := c.GRPC(grpcOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("serve: build grpc server: %w", err)
	}

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// Reflection exposes every service schema to any network client, so it
	// stays off unless GRPC_REFLECTION opts in or ZEVER_ENV is dev.
	if reflectionEnabled() {
		reflection.Register(grpcServer)
	}

	return grpcServer, healthServer, nil
}

// rateLimitUnaryInterceptor returns the rate-limit interceptor when a
// limiter is configured, or nil (no interception) when it is not. A nil
// grpc.UnaryServerInterceptor is a no-op in ChainUnaryInterceptor.
func rateLimitUnaryInterceptor(limiter ratelimit.Limiter) grpc.UnaryServerInterceptor {
	if limiter == nil {
		return nil
	}

	return middleware.RateLimitUnaryServerInterceptor(limiter, middleware.PeerAddrKey)
}
