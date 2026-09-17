package storage

import (
	"github.com/zenta-dev/zever/log"
)

// LocalOptions configures the local-filesystem backend.
type LocalOptions struct {
	// Root is the directory objects are stored under.
	Root string
	// Secret is the HMAC key for presigned URLs.
	Secret string
	// PublicURL is the public base URL for unsigned links.
	PublicURL string
}

// S3Options configures the Amazon S3 backend.
type S3Options struct {
	// AccessKeyID is the AWS access key.
	AccessKeyID string
	// SecretAccessKey is the AWS secret key.
	SecretAccessKey string
	// Region is the AWS region, defaulting to us-east-1 when empty.
	Region string
	// Endpoint overrides the S3 endpoint (S3-compatible stores).
	Endpoint string
	// PolicySync is manual or auto for bucket-policy sync.
	PolicySync string
	// SyncFail is warn or require on sync failure.
	SyncFail string
}

// R2Options configures the Cloudflare R2 backend.
type R2Options struct {
	// AccountID is the Cloudflare account ID.
	AccountID string
}

// Options configures storage backend selection and backend-specific settings.
type Options struct {
	// URLBase is the base URL minted presigned URLs build on.
	URLBase string
	// Policy is the typed access policy; nil means legacy private mode.
	Policy *PolicyConfig

	// LocalOptions holds local-backend settings.
	LocalOptions
	// S3Options holds S3-backend settings.
	S3Options
	// R2Options holds R2-backend settings.
	R2Options
	// Logger emits backend warnings. Defaults to a no-op logger when nil.
	Logger log.Logger
}

// Validate checks the backend-agnostic fields of Options.
func (o Options) Validate() error {
	if err := ValidateBaseURL("storage", "url_base", o.URLBase); err != nil {
		return err
	}

	if o.Policy != nil {
		if _, _, _, err := o.Policy.Resolve(); err != nil {
			return err
		}
	}

	return nil
}
