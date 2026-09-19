package crypto

import (
	"encoding/base64"
	"errors"
)

// Options holds typed configuration for crypto adapters.
type Options struct {
	// Key is the base64-encoded 32-byte encryption key. Rotating Key makes
	// every value previously encrypted under the old key permanently
	// undecryptable: adapters (e.g. crypto/local) embed no key-id/version
	// in their output. Plan key rotation accordingly (see crypto/local's
	// Encrypt doc comment).
	Key string `json:"key" toml:"key" yaml:"key"`
	// SignKey is the base64-encoded 64-byte signing key.
	SignKey string `json:"sign_key" toml:"sign_key" yaml:"sign_key"`
}

// Validate checks required fields and key formats, joining all violations.
//
// Deviation from dirty local/options.Validate: the original only validated
// Key as 32 bytes; this core facade centralizes validation and now checks
// both Key (required, 32 bytes) and SignKey (optional, 64 bytes when present)
// so adapters need not repeat secret-format checks.
func (o Options) Validate() error {
	var errs []error

	if o.Key == "" {
		errs = append(errs, &InvalidOptionsError{Reason: "key is required"})
	} else {
		b, err := base64.StdEncoding.DecodeString(o.Key)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "key must be valid base64"})
		} else if len(b) != 32 {
			errs = append(errs, &InvalidOptionsError{Reason: "key must decode to 32 bytes"})
		}
	}

	if o.SignKey != "" {
		b, err := base64.StdEncoding.DecodeString(o.SignKey)
		if err != nil {
			errs = append(errs, &InvalidOptionsError{Reason: "sign_key must be valid base64"})
		} else if len(b) != 64 {
			errs = append(errs, &InvalidOptionsError{Reason: "sign_key must decode to 64 bytes"})
		}
	}

	return errors.Join(errs...)
}
