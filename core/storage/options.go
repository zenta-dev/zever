package storage

import (
	"github.com/zenta-dev/zever/core/log"
)

// LocalOptions configures the local-filesystem backend.
type LocalOptions struct {
	// Root is the directory objects are stored under.
	Root string `json:"root"`
	// Secret is the HMAC key for presigned URLs.
	Secret string `json:"secret" zever:"secret"`
	// PublicURL is the public base URL for unsigned links.
	PublicURL string `json:"public_url"`
	// MaxUploadBytes caps the size of one uploaded object; zero means the
	// adapter default (64 MiB for the local backend).
	MaxUploadBytes int64 `json:"max_upload_bytes"`
}

// S3Options configures the Amazon S3 backend.
type S3Options struct {
	// AccessKeyID is the AWS access key.
	AccessKeyID string `json:"access_key_id" zever:"secret"`
	// SecretAccessKey is the AWS secret key.
	SecretAccessKey string `json:"secret_access_key" zever:"secret"`
	// Region is the AWS region, defaulting to us-east-1 when empty.
	Region string `json:"region"`
	// Endpoint overrides the S3 endpoint (S3-compatible stores).
	Endpoint string `json:"endpoint"`
	// PolicySync is manual or auto for bucket-policy sync.
	PolicySync string `json:"policy_sync"`
	// SyncFail is warn or require on sync failure.
	SyncFail string `json:"sync_fail"`
}

// R2Options configures the Cloudflare R2 backend.
type R2Options struct {
	// AccountID is the Cloudflare account ID.
	AccountID string `json:"account_id"`
}

// Options configures storage backend selection and backend-specific settings.
type Options struct {
	// URLBase is the base URL minted presigned URLs build on.
	URLBase string `json:"url_base"`
	// Policy is the typed access policy; nil means legacy private mode.
	Policy *PolicyConfig `json:"policy"`

	// LocalOptions holds local-backend settings.
	LocalOptions
	// S3Options holds S3-backend settings.
	S3Options
	// R2Options holds R2-backend settings.
	R2Options
	// Logger emits backend warnings. Defaults to a no-op logger when nil.
	Logger log.Logger `json:"-"`
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	if err := ValidateBaseURL("storage", "url_base", o.URLBase); err != nil {
		return InvalidOptionsError{Reason: err.Error()}
	}

	if o.Policy != nil {
		if _, _, _, err := o.Policy.Resolve(); err != nil {
			return InvalidOptionsError{Reason: err.Error()}
		}
	}

	return nil
}
