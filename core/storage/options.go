package storage

import (
	"github.com/zenta-dev/zever/core/log"
)

// LocalOptions configures the local-filesystem backend.
type LocalOptions struct {
	// Root is the directory objects are stored under.
	Root string `json:"root" toml:"root" yaml:"root"`
	// Secret is the HMAC key for presigned URLs.
	Secret string `json:"secret" toml:"secret" yaml:"secret"`
	// PublicURL is the public base URL for unsigned links.
	PublicURL string `json:"public_url" toml:"public_url" yaml:"public_url"`
}

// S3Options configures the Amazon S3 backend.
type S3Options struct {
	// AccessKeyID is the AWS access key.
	AccessKeyID string `json:"access_key_id" toml:"access_key_id" yaml:"access_key_id"`
	// SecretAccessKey is the AWS secret key.
	SecretAccessKey string `json:"secret_access_key" toml:"secret_access_key" yaml:"secret_access_key"`
	// Region is the AWS region, defaulting to us-east-1 when empty.
	Region string `json:"region" toml:"region" yaml:"region"`
	// Endpoint overrides the S3 endpoint (S3-compatible stores).
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// PolicySync is manual or auto for bucket-policy sync.
	PolicySync string `json:"policy_sync" toml:"policy_sync" yaml:"policy_sync"`
	// SyncFail is warn or require on sync failure.
	SyncFail string `json:"sync_fail" toml:"sync_fail" yaml:"sync_fail"`
}

// R2Options configures the Cloudflare R2 backend.
type R2Options struct {
	// AccountID is the Cloudflare account ID.
	AccountID string `json:"account_id" toml:"account_id" yaml:"account_id"`
}

// Options configures storage backend selection and backend-specific settings.
type Options struct {
	// URLBase is the base URL minted presigned URLs build on.
	URLBase string `json:"url_base" toml:"url_base" yaml:"url_base"`
	// Policy is the typed access policy; nil means legacy private mode.
	Policy *PolicyConfig `json:"policy" toml:"policy" yaml:"policy"`

	// LocalOptions holds local-backend settings.
	LocalOptions
	// S3Options holds S3-backend settings.
	S3Options
	// R2Options holds R2-backend settings.
	R2Options
	// Logger emits backend warnings. Defaults to a no-op logger when nil.
	Logger log.Logger `json:"-" toml:"-" yaml:"-"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	if err := ValidateBaseURL("storage", "url_base", o.URLBase); err != nil {
		return &InvalidOptionsError{Reason: err.Error()}
	}

	if o.Policy != nil {
		if _, _, _, err := o.Policy.Resolve(); err != nil {
			return &InvalidOptionsError{Reason: err.Error()}
		}
	}

	return nil
}
