package ai

import (
	"errors"
	"log/slog"
	"time"

	"github.com/zenta-dev/zever/shared/endpoint"
)

// Options configures the AI backend.
type Options struct {
	APIKey  string `json:"api_key" zever:"secret"`
	Model   string `json:"model"`
	BaseURL string `json:"base_url"`
	// AllowInsecure permits an http BaseURL for testing. Default false (https
	// only). AllowInsecure no longer disables TLS verification; set
	// InsecureSkipVerify explicitly for self-signed test servers.
	AllowInsecure bool `json:"allow_insecure"`
	// AllowPrivate permits dialing private, loopback, and link-local
	// addresses (required for a local Ollama/sidecar BaseURL). Default
	// false: only globally routable addresses are dialed, redirects are
	// never followed, and the adapter logs a line at open when enabled.
	AllowPrivate bool `json:"allow_private"`
	// InsecureSkipVerify disables TLS certificate verification for
	// self-signed test servers. Default false: verification always on.
	// Enable only for testing; the adapter logs a warning at open.
	InsecureSkipVerify bool          `json:"insecure_skip_verify"`
	Timeout            time.Duration `json:"timeout"`
	// Logger receives the adapter's open-time notices (allow_private and
	// insecure_skip_verify). Nil uses slog.Default(). It is never loaded
	// from configuration and never validated.
	Logger *slog.Logger `json:"-"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if o.Timeout < 0 {
		errs = append(errs, InvalidOptionsError{Reason: "timeout must be >= 0"})
	}

	if o.BaseURL != "" {
		if _, err := endpoint.ValidateURL(o.BaseURL, endpoint.WithAllowInsecure(o.AllowInsecure)); err != nil {
			errs = append(errs, InvalidOptionsError{Reason: baseURLReason(err)})
		}
	}

	return errors.Join(errs...)
}

// baseURLReason maps endpoint validation failures to the historical
// base_url reason strings.
func baseURLReason(err error) string {
	switch {
	case errors.Is(err, endpoint.ErrParse):
		return "base_url must be a valid url"
	case errors.Is(err, endpoint.ErrNoScheme):
		return "base_url must include scheme"
	case errors.Is(err, endpoint.ErrNoHost):
		return "base_url must include host"
	default:
		return "base_url must use https scheme"
	}
}
