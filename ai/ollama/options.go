package ollama

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultAddr is the Ollama server address used when Options.Addr is empty.
const DefaultAddr = "http://localhost:11434"

// DefaultTimeout bounds whole HTTP exchanges when Options.Timeout is not positive.
const DefaultTimeout = 60 * time.Second

// Options configures the Ollama adapter.
type Options struct {
	// Addr is the Ollama server base URL. Empty means DefaultAddr.
	Addr string
	// Model is the default model when callers pass an empty model.
	Model string
	// Timeout bounds each HTTP exchange. Non-positive means DefaultTimeout.
	Timeout time.Duration
	// Transport overrides the HTTP round tripper. Nil means http.DefaultTransport.
	// Tests use a hand-fake transport; production leaves it nil.
	Transport http.RoundTripper
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
	if strings.Contains(addr, " ") || strings.Contains(addr, "\n") ||
		strings.Contains(addr, "\t") || strings.Contains(addr, "\r") ||
		strings.Contains(addr, "\v") {
		return errors.New("ollama: addr must not contain whitespace")
	}

	u, err := url.Parse(addr)
	if err != nil {
		return errors.New("ollama: addr must be a valid URL")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("ollama: addr must have http or https scheme")
	}

	if u.Host == "" {
		return errors.New("ollama: addr must have a host")
	}

	if u.User != nil {
		return errors.New("ollama: addr must not contain user info")
	}

	return nil
}
