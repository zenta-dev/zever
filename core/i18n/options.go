package i18n

import (
	"errors"
	"io/fs"
	"time"

	"github.com/zenta-dev/zever/shared/endpoint"
)

// DefaultTimeout is the default i18n operation timeout applied by adapters.
const DefaultTimeout = 30 * time.Second

// EmbedOptions configures the embedded-catalog adapter.
type EmbedOptions struct {
	// FS is the file system holding message catalogs.
	FS fs.FS `json:"-" toml:"-" yaml:"-"`
	// Dir is the catalog directory. Empty means the adapter default (".").
	Dir string `json:"dir" toml:"dir" yaml:"dir"`
	// Fallback is the fallback locale. Empty means none.
	Fallback string `json:"fallback" toml:"fallback" yaml:"fallback"`
}

// RemoteOptions configures the remote-service adapter.
type RemoteOptions struct {
	// Endpoint is the remote service base URL. Empty means unconfigured.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// APIKey is the remote service credential.
	APIKey string `json:"api_key" toml:"api_key" yaml:"api_key"`
	// Timeout is the operation timeout. Zero means the default.
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
	// MaxInFlight caps concurrent requests. Zero means adapter default (1000).
	MaxInFlight int `json:"max_in_flight" toml:"max_in_flight" yaml:"max_in_flight"`
	// AllowInsecure permits http:// endpoints. Default false (https only).
	AllowInsecure bool `json:"allow_insecure" toml:"allow_insecure" yaml:"allow_insecure"`
}

// Options configures backend construction.
type Options struct {
	// Embed carries the embedded-catalog adapter settings.
	Embed EmbedOptions `json:"embed" toml:"embed" yaml:"embed"`
	// Remote carries the remote-service adapter settings.
	Remote RemoteOptions `json:"remote" toml:"remote" yaml:"remote"`
}

// Validate checks options for consistency, joining all violations.
//
// Validate checks options for consistency.
// Empty Remote.Endpoint is valid; otherwise it must be a URL with scheme
// and host, http allowed only when AllowInsecure is set.
// Zero Timeout means "apply default" and is valid; only negative values fail.
// Zero MaxInFlight means "adapter default" and is valid; only negative values fail.
// Empty Embed.Dir and Embed.Fallback are valid (adapter defaults apply).
func (o Options) Validate() error {
	if o.Remote.Timeout < 0 {
		return &InvalidOptionsError{Reason: "timeout must be >= 0"}
	}
	if o.Remote.MaxInFlight < 0 {
		return &InvalidOptionsError{Reason: "max_in_flight must be >= 0"}
	}
	if o.Remote.Endpoint == "" {
		return nil
	}
	if _, err := endpoint.ValidateURL(o.Remote.Endpoint, endpoint.WithAllowInsecure(o.Remote.AllowInsecure)); err != nil {
		switch {
		case errors.Is(err, endpoint.ErrParse):
			return &InvalidOptionsError{Reason: "endpoint must be a valid url"}
		case errors.Is(err, endpoint.ErrNoScheme),
			errors.Is(err, endpoint.ErrNoHost),
			errors.Is(err, endpoint.ErrEmpty):
			return &InvalidOptionsError{Reason: "endpoint must have scheme and host"}
		case errors.Is(err, endpoint.ErrInsecureScheme):
			return &InvalidOptionsError{Reason: "http endpoint requires allow_insecure"}
		default:
			return &InvalidOptionsError{Reason: "endpoint scheme must be https"}
		}
	}
	return nil
}

// timeout returns the effective timeout, defaulting to DefaultTimeout.
func (o Options) timeout() time.Duration {
	if o.Remote.Timeout == 0 {
		return DefaultTimeout
	}
	return o.Remote.Timeout
}
