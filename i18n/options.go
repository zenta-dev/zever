package i18n

import (
	"io/fs"
	"net/url"
	"time"
)

// DefaultTimeout is the default i18n operation timeout applied by adapters.
const DefaultTimeout = 30 * time.Second

// EmbedOptions configures the embedded-catalog adapter.
type EmbedOptions struct {
	// FS is the file system holding message catalogs.
	FS fs.FS
	// Dir is the catalog directory. Empty means the adapter default (".").
	Dir string
	// Fallback is the fallback locale. Empty means none.
	Fallback string
}

// RemoteOptions configures the remote-service adapter.
type RemoteOptions struct {
	// Endpoint is the remote service base URL. Empty means unconfigured.
	Endpoint string
	// APIKey is the remote service credential.
	APIKey string
	// Timeout is the operation timeout. Zero means the default.
	Timeout time.Duration
	// MaxInFlight caps concurrent requests. Zero means adapter default (1000).
	MaxInFlight int
	// AllowInsecure permits http:// endpoints. Default false (https only).
	AllowInsecure bool
}

// Options configures backend construction.
type Options struct {
	// Embed carries the embedded-catalog adapter settings.
	Embed EmbedOptions
	// Remote carries the remote-service adapter settings.
	Remote RemoteOptions
}

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
		return &InvalidOptionsError{Reason: "maxinflight must be >= 0"}
	}
	if o.Remote.Endpoint == "" {
		return nil
	}
	u, err := url.Parse(o.Remote.Endpoint)
	if err != nil {
		return &InvalidOptionsError{Reason: "endpoint must be a valid URL"}
	}
	if u.Scheme == "" || u.Host == "" {
		return &InvalidOptionsError{Reason: "endpoint must have scheme and host"}
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !o.Remote.AllowInsecure {
			return &InvalidOptionsError{Reason: "http endpoint requires allowinsecure"}
		}
	default:
		return &InvalidOptionsError{Reason: "endpoint scheme must be https"}
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
