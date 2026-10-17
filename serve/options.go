package serve

import (
	"github.com/zenta-dev/zever/config"
)

// DefaultPort is the HTTP listen port when neither WithPort nor PORT sets one.
const DefaultPort = "8080"

// options is the folded form of one Main or Worker call's Options.
type options struct {
	port      string
	grpcAddr  string
	pprofAddr string
	env       config.Env
	hasEnv    bool
	onListen  func(network, addr string)
}

// Option customizes one Main or Worker call. Explicit options win over the
// process environment, which wins over the defaults. Options are per-call
// values; a nil Option is ignored.
type Option func(*options)

// WithPort sets the HTTP listen port, overriding PORT and DefaultPort.
// An empty port falls back to PORT, then DefaultPort.
func WithPort(port string) Option {
	return func(o *options) {
		if port != "" {
			o.port = port
		}
	}
}

// WithGRPCAddr sets the gRPC listen address (for example ":9090"),
// overriding GRPC_ADDR. Empty (the default) disables the gRPC listener:
// modules declaring GRPCPolicies then fail closed at boot.
func WithGRPCAddr(addr string) Option {
	return func(o *options) {
		o.grpcAddr = addr
	}
}

// WithPprofAddr sets the loopback pprof listen address, overriding
// PPROF_ADDR. Empty (the default) disables profiling. Non-loopback
// addresses fail closed at boot.
func WithPprofAddr(addr string) Option {
	return func(o *options) {
		o.pprofAddr = addr
	}
}

// WithEnv replaces the environment serve resolves config from,
// instead of config.FromEnv. Tests pass literal maps; production leaves
// this unset. Provider overlays inside container still read the process
// environment (see container.New), so provider tests use t.Setenv for
// SERVICE_* variables.
func WithEnv(env config.Env) Option {
	return func(o *options) {
		o.env = env
		o.hasEnv = true
	}
}

// WithEnvOverride is an alias of WithEnv kept for call sites written
// against the pre-release name.
func WithEnvOverride(env config.Env) Option {
	return WithEnv(env)
}

// withListenHook reports each bound listener address. It is a test seam:
// production call sites never set it.
func withListenHook(fn func(network, addr string)) Option {
	return func(o *options) {
		o.onListen = fn
	}
}

// foldOptions merges one call's Options: later values win, nils ignored.
func foldOptions(opts []Option) options {
	var o options

	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	return o
}

// httpPort resolves the HTTP listen port: explicit option, then PORT,
// then DefaultPort.
func (o options) httpPort() string {
	if o.port != "" {
		return o.port
	}

	if v, ok := lookupEnv("PORT"); ok && v != "" {
		return v
	}

	return DefaultPort
}

// grpcAddr resolves the gRPC listen address: explicit option, then
// GRPC_ADDR, then empty (disabled).
func (o options) grpcAddrResolved() string {
	if o.grpcAddr != "" {
		return o.grpcAddr
	}

	if v, ok := lookupEnv("GRPC_ADDR"); ok {
		return v
	}

	return ""
}

// pprofAddr resolves the pprof listen address: explicit option, then
// PPROF_ADDR, then empty (disabled).
func (o options) pprofAddrResolved() string {
	if o.pprofAddr != "" {
		return o.pprofAddr
	}

	if v, ok := lookupEnv("PPROF_ADDR"); ok {
		return v
	}

	return ""
}
