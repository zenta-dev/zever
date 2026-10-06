package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/resilience"
)

// DefaultMaxAttempts is the client-side cap for RetryPolicy.MaxAttempts,
// matching gRPC's own limit for retry maxAttempts.
const DefaultMaxAttempts = 5

// config holds New options.
type config struct {
	insecure         bool
	tls              *TLSOptions
	guard            resilience.Guard
	timeout          time.Duration
	timeoutSet       bool
	retry            *RetryPolicy
	serviceConfig    string
	serviceConfigSet bool
	metadata         map[string]string
	observability    observability.Provider
	staticResolver   *staticResolverBuilder
}

// Option configures New.
type Option func(*config)

// WithInsecure disables transport credentials. WithInsecure is for
// development and loopback only; it is mutually exclusive with WithTLS.
func WithInsecure() Option {
	return func(c *config) { c.insecure = true }
}

// WithTLS enables TLS transport credentials from o. CertFile/KeyFile form
// an optional client certificate pair (both or neither), CAFile overrides
// the root CA pool used to verify the server, and ServerName overrides the
// certificate verification name. Read or parse failures are returned as
// errors; WithTLS never falls back to insecure.
func WithTLS(o TLSOptions) Option {
	return func(c *config) { c.tls = &o }
}

// WithGuard routes every RPC through the resilience.Guard (breaker,
// bulkhead, retry composition). A nil guard is ignored. For streams the
// guard wraps stream establishment only.
func WithGuard(g resilience.Guard) Option {
	return func(c *config) { c.guard = g }
}

// WithTimeout applies a per-RPC deadline on top of any caller context.
// The deadline governs the whole call, including guard retries.
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		c.timeout = d
		c.timeoutSet = true
	}
}

// WithRetryPolicy merges a per-method retry policy into the gRPC service
// config. MaxAttempts is capped at DefaultMaxAttempts; invalid policies
// are rejected with an error rather than silently ignored.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *config) { c.retry = &p }
}

// WithServiceConfigJSON merges raw over the default service config:
// top-level keys in raw (for example loadBalancingConfig or methodConfig)
// replace the defaults, while keys absent from raw keep the defaults.
// Invalid JSON is rejected with an error.
func WithServiceConfigJSON(raw string) Option {
	return func(c *config) {
		c.serviceConfig = raw
		c.serviceConfigSet = true
	}
}

// WithMetadata attaches the given keys to outgoing RPC metadata. Keys
// already present in the outgoing context are never overwritten.
func WithMetadata(md map[string]string) Option {
	return func(c *config) { c.metadata = md }
}

// WithObservability starts a client span per RPC when p is non-nil.
// Spans cover the whole call, including guard execution.
func WithObservability(p observability.Provider) Option {
	return func(c *config) { c.observability = p }
}

// WithStaticResolver installs a per-connection resolver for static:///name
// targets. addrs maps a name to one or more server addresses; multiple
// addresses per name spread load under the default round_robin policy.
// The resolver is registered via grpc.WithResolvers, never globally.
func WithStaticResolver(addrs map[string][]string) Option {
	return func(c *config) { c.staticResolver = &staticResolverBuilder{addrs: addrs} }
}

// TLSOptions configures client TLS credentials for WithTLS.
type TLSOptions struct {
	// CertFile is the PEM-encoded client certificate file path. CertFile
	// and KeyFile must both be set or both empty.
	CertFile string
	// KeyFile is the PEM-encoded client private key file path.
	KeyFile string
	// CAFile is the PEM-encoded CA bundle used to verify the server.
	CAFile string
	// ServerName overrides the server name used for certificate
	// verification.
	ServerName string
}

// New returns a *grpc.ClientConn for target configured by opts. New
// performs no IO: the connection is dialed lazily on the first RPC and is
// closed by the caller.
//
// Target schemes: dns:///host:port uses gRPC's built-in DNS resolver;
// static:///name resolves through the WithStaticResolver address map.
//
// Credentials are explicit: New returns ErrNoCredentials unless WithInsecure
// or WithTLS is passed, and ErrInsecureAndTLS when both are.
func New(ctx context.Context, target string, opts ...Option) (*grpc.ClientConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("grpcclient: context: %w", err)
	}

	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	creds, err := cfg.transportCredentials()
	if err != nil {
		return nil, err
	}

	if cfg.timeoutSet && cfg.timeout <= 0 {
		return nil, fmt.Errorf("grpcclient: timeout must be positive, got %v", cfg.timeout)
	}

	scJSON, err := cfg.serviceConfigJSON()
	if err != nil {
		return nil, err
	}

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultServiceConfig(scJSON),
		grpc.WithChainUnaryInterceptor(cfg.unaryInterceptor()),
		grpc.WithChainStreamInterceptor(cfg.streamInterceptor()),
	}
	if cfg.staticResolver != nil {
		dialOpts = append(dialOpts, grpc.WithResolvers(cfg.staticResolver))
	}

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: dial %q: %w", target, err)
	}

	return conn, nil
}

// transportCredentials resolves the credentials dial option from the
// configured options, failing closed when neither or both are set.
func (c *config) transportCredentials() (credentials.TransportCredentials, error) {
	switch {
	case c.insecure && c.tls != nil:
		return nil, ErrInsecureAndTLS
	case c.insecure:
		return insecure.NewCredentials(), nil
	case c.tls != nil:
		return c.tls.credentials()
	default:
		return nil, ErrNoCredentials
	}
}

// credentials builds TLS transport credentials from the options, failing
// closed on read or parse errors.
func (o TLSOptions) credentials() (credentials.TransportCredentials, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if o.ServerName != "" {
		tlsCfg.ServerName = o.ServerName
	}

	if o.CertFile != "" || o.KeyFile != "" {
		if o.CertFile == "" || o.KeyFile == "" {
			return nil, errors.New("grpcclient: TLSOptions: CertFile and KeyFile must both be set for a client certificate")
		}
		cert, err := tls.LoadX509KeyPair(o.CertFile, o.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("grpcclient: load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	if o.CAFile != "" {
		pem, err := os.ReadFile(o.CAFile)
		if err != nil {
			return nil, fmt.Errorf("grpcclient: read CA file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("grpcclient: no certificates found in CA file %q", o.CAFile)
		}
		tlsCfg.RootCAs = pool
	}

	return credentials.NewTLS(tlsCfg), nil
}
