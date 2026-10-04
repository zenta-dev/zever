package crypto

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
)

// Options holds typed configuration for crypto adapters.
type Options struct {
	// Key is the base64-encoded 32-byte encryption key. Rotating Key makes
	// every value previously encrypted under the old key permanently
	// undecryptable: adapters (e.g. crypto/local) embed no key-id/version
	// in their output. Plan key rotation accordingly (see crypto/local's
	// Encrypt doc comment).
	//
	// Key is required for the local adapter. For the kms adapter it is
	// optional: envelope encryption uses data keys from KMS, and Key (when
	// present) only backs MAC computation. When any KMS field is set
	// (KeyID, KeyIDs, Region, Endpoint, UseEnvelope), Key may be empty.
	Key string `json:"key" toml:"key" yaml:"key"`
	// SignKey is the base64-encoded 64-byte signing key.
	SignKey string `json:"sign_key" toml:"sign_key" yaml:"sign_key"`
	// KeyID is the primary KMS key identifier used for envelope encryption.
	KeyID string `json:"key_id" toml:"key_id" yaml:"key_id"`
	// KeyIDs lists accepted KMS key identifiers for rotation. Encrypt uses
	// KeyID; Decrypt accepts envelopes sealed under KeyID or any KeyIDs entry.
	KeyIDs []string `json:"key_ids" toml:"key_ids" yaml:"key_ids"`
	// Region is the KMS region (e.g. AWS region).
	Region string `json:"region" toml:"region" yaml:"region"`
	// Endpoint overrides the KMS endpoint (custom KMS, tests, VPC endpoints).
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// UseEnvelope selects KMS envelope encryption. When true, KeyID is required.
	UseEnvelope bool `json:"use_envelope" toml:"use_envelope" yaml:"use_envelope"`
}

// Validate checks options for consistency, joining all violations.
//
// Validate checks required fields and key formats, joining all violations.
//
// Deviation from dirty local/options.Validate: the original only validated
// Key as 32 bytes; this core facade centralizes validation and now checks
// both Key (required, 32 bytes) and SignKey (optional, 64 bytes when present)
// so adapters need not repeat secret-format checks.
//
// KMS fields (KeyID, KeyIDs, Region, Endpoint, UseEnvelope) are optional.
// When any is set, Key becomes optional (envelope data keys come from KMS);
// when none is set, Key stays required for the local adapter. UseEnvelope
// requires KeyID; Endpoint must be a valid URL with scheme and host.
func (o Options) Validate() error {
	var errs []error

	isKMS := o.KeyID != "" || len(o.KeyIDs) > 0 || o.Region != "" || o.Endpoint != "" || o.UseEnvelope

	if o.Key == "" {
		if !isKMS {
			errs = append(errs, InvalidOptionsError{Reason: "key is required"})
		}
	} else {
		b, err := base64.StdEncoding.DecodeString(o.Key)
		if err != nil {
			errs = append(errs, InvalidOptionsError{Reason: "key must be valid base64"})
		} else if len(b) != 32 {
			errs = append(errs, InvalidOptionsError{Reason: "key must decode to 32 bytes"})
		}
	}

	if o.SignKey != "" {
		b, err := base64.StdEncoding.DecodeString(o.SignKey)
		if err != nil {
			errs = append(errs, InvalidOptionsError{Reason: "sign_key must be valid base64"})
		} else if len(b) != 64 {
			errs = append(errs, InvalidOptionsError{Reason: "sign_key must decode to 64 bytes"})
		}
	}

	if o.UseEnvelope && strings.TrimSpace(o.KeyID) == "" {
		errs = append(errs, InvalidOptionsError{Reason: "key_id is required when use_envelope is true"})
	}

	for _, id := range o.KeyIDs {
		if strings.TrimSpace(id) == "" {
			errs = append(errs, InvalidOptionsError{Reason: "key_ids must not contain empty entries"})
			break
		}
	}

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil || u.Scheme == "" {
			errs = append(errs, InvalidOptionsError{Reason: "endpoint must include scheme"})
		}
		if err == nil && u.Host == "" {
			errs = append(errs, InvalidOptionsError{Reason: "endpoint must include host"})
		}
	}

	return errors.Join(errs...)
}
