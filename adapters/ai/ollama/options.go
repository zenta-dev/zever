package ollama

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/zenta-dev/zever/shared/endpoint"
)

// DefaultAddr is the Ollama server address used when Options.Addr is empty.
const DefaultAddr = "http://localhost:11434"

// DefaultTimeout bounds whole HTTP exchanges when Options.Timeout is not positive.
const DefaultTimeout = 60 * time.Second

// Options configures the Ollama adapter.
type Options struct {
	// Addr is the Ollama server base URL. Empty means DefaultAddr.
	Addr string `json:"addr" toml:"addr" yaml:"addr"`
	// Model is the default model when callers pass an empty model.
	Model string `json:"model" toml:"model" yaml:"model"`
	// Timeout bounds each HTTP exchange. Non-positive means DefaultTimeout.
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
	// AllowInsecure permits a plain-http Addr for non-loopback hosts.
	// Default false: https required for non-loopback hosts. Loopback http
	// (the DefaultAddr host) is always permitted for local development.
	// AllowInsecure no longer disables TLS verification; set
	// InsecureSkipVerify explicitly for self-signed test servers.
	AllowInsecure bool `json:"allow_insecure" toml:"allow_insecure" yaml:"allow_insecure"`
	// AllowPrivate permits dialing private, loopback, and link-local
	// addresses. Default false, so the loopback DefaultAddr needs an
	// explicit opt-in: only globally routable addresses are dialed,
	// redirects are never followed, and the adapter logs a line at open
	// when enabled.
	AllowPrivate bool `json:"allow_private" toml:"allow_private" yaml:"allow_private"`
	// InsecureSkipVerify disables TLS certificate verification for
	// self-signed test servers. Default false: verification always on.
	// Enable only for testing; the adapter logs a warning at open.
	InsecureSkipVerify bool `json:"insecure_skip_verify" toml:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	// Transport overrides the HTTP round tripper. Nil means http.DefaultTransport.
	// Tests use a hand-fake transport; production leaves it nil.
	Transport http.RoundTripper `json:"-" toml:"-" yaml:"-"`
	// Logger receives the open-time notices for AllowPrivate and
	// InsecureSkipVerify. Nil means slog.Default().
	Logger *slog.Logger `json:"-" toml:"-" yaml:"-"`
}

// ErrInvalidAddr is returned when Options.Addr fails endpoint validation.
// The wrapped endpoint error names the failure shape; addrReason preserves
// the historical fragments callers match on.
var ErrInvalidAddr = errors.New("ollama: invalid addr")

// Validate checks Options for consistency, joining all violations.
// Secret values are never echoed: only the address shape is reported.
func (o Options) Validate() error {
	var errs []error

	if o.Timeout < 0 {
		errs = append(errs, fmt.Errorf("ollama: timeout must be >= 0: %w", ErrInvalidTimeout))
	}

	if o.Addr != "" {
		if err := validateAddr(o.Addr, o.AllowInsecure); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// validateAddr rejects addresses that are not plain http(s) URLs
// without user info or whitespace. Plain http is permitted only for
// loopback hosts (local development) unless AllowInsecure opts in.
func validateAddr(addr string, allowInsecure bool) error {
	if _, err := endpoint.ValidateURL(addr,
		endpoint.WithAllowInsecure(allowInsecure),
		endpoint.WithAllowLoopbackHTTP(),
		endpoint.WithRejectUserinfo(),
		endpoint.WithRejectWhitespace(),
	); err != nil {
		return errors.Join(fmt.Errorf("%w: %s", ErrInvalidAddr, addrReason(err)), err)
	}

	return nil
}

// addrReason maps endpoint validation failures to the historical addr
// shape fragments.
func addrReason(err error) string {
	switch {
	case errors.Is(err, endpoint.ErrWhitespace):
		return "must not contain whitespace"
	case errors.Is(err, endpoint.ErrParse):
		return "must be a valid URL"
	case errors.Is(err, endpoint.ErrNoHost):
		return "must have a host"
	case errors.Is(err, endpoint.ErrUserinfo):
		return "must not contain user info"
	default:
		return "must have http or https scheme"
	}
}
