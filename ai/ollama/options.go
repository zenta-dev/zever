package ollama

import (
	"errors"
	"net/http"
	"time"

	"github.com/zenta-dev/zever/internal/endpoint"
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
	// Transport overrides the HTTP round tripper. Nil means http.DefaultTransport.
	// Tests use a hand-fake transport; production leaves it nil.
	Transport http.RoundTripper `json:"-" toml:"-" yaml:"-"`
}

// Validate checks Options for consistency, joining all violations.
// Secret values are never echoed: only the address shape is reported.
func (o Options) Validate() error {
	var errs []error

	if o.Timeout < 0 {
		errs = append(errs, errors.New("ollama: timeout must be >= 0"))
	}

	if o.Addr != "" {
		if err := validateAddr(o.Addr); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// validateAddr rejects addresses that are not plain http(s) URLs
// without user info or whitespace.
func validateAddr(addr string) error {
	if _, err := endpoint.ValidateURL(addr,
		endpoint.WithAllowInsecure(true),
		endpoint.WithRejectUserinfo(),
		endpoint.WithRejectWhitespace(),
	); err != nil {
		return errors.New("ollama: addr " + addrReason(err))
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
