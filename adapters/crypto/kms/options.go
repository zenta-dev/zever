package kms

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
)

// Options holds typed configuration for the kms crypto adapter.
type Options struct {
	// Key is the optional base64-encoded 32-byte key backing MAC computation.
	Key string `json:"key" toml:"key" yaml:"key"`
	// SignKey is the optional base64-encoded 64-byte signing key.
	SignKey string `json:"sign_key" toml:"sign_key" yaml:"sign_key"`
	// KeyID is the primary KMS key identifier. Required.
	KeyID string `json:"key_id" toml:"key_id" yaml:"key_id"`
	// KeyIDs lists accepted KMS key identifiers for rotation.
	KeyIDs []string `json:"key_ids" toml:"key_ids" yaml:"key_ids"`
	// Region is the KMS region.
	Region string `json:"region" toml:"region" yaml:"region"`
	// Endpoint overrides the KMS endpoint.
	Endpoint string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	// UseEnvelope selects envelope encryption. Always true for this adapter;
	// kept for parity with core crypto.Options.
	UseEnvelope bool `json:"use_envelope" toml:"use_envelope" yaml:"use_envelope"`
	// DevStub opts New into the deterministic no-boundary stub client.
	// Test/dev only: New fails closed without it, so a production config
	// selecting this adapter can never silently lose the KMS boundary.
	DevStub bool `json:"dev_stub" toml:"dev_stub" yaml:"dev_stub"`
}

// Validate checks Options for consistency, joining all violations.
func (o Options) Validate() error {
	var errs []error

	if strings.TrimSpace(o.KeyID) == "" {
		errs = append(errs, errors.New("kms: key_id is required"))
	} else if len(o.KeyID) > 65535 {
		errs = append(errs, errors.New("kms: key_id too long"))
	}

	for _, id := range o.KeyIDs {
		if strings.TrimSpace(id) == "" {
			errs = append(errs, errors.New("kms: key_ids must not contain empty entries"))
			break
		}
		if len(id) > 65535 {
			errs = append(errs, errors.New("kms: key_ids must not contain overlong entries"))
			break
		}
	}

	if o.Key != "" {
		b, err := base64.StdEncoding.DecodeString(o.Key)
		if err != nil {
			errs = append(errs, errors.New("kms: key must be valid base64"))
		} else if len(b) != 32 {
			errs = append(errs, errors.New("kms: key must decode to 32 bytes"))
		}
	}

	if o.SignKey != "" {
		b, err := base64.StdEncoding.DecodeString(o.SignKey)
		if err != nil {
			errs = append(errs, errors.New("kms: sign_key must be valid base64"))
		} else if len(b) != 64 {
			errs = append(errs, errors.New("kms: sign_key must decode to 64 bytes"))
		}
	}

	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil || u.Scheme == "" {
			errs = append(errs, errors.New("kms: endpoint must include scheme"))
		}
		if err == nil && u.Host == "" {
			errs = append(errs, errors.New("kms: endpoint must include host"))
		}
	}

	return errors.Join(errs...)
}
