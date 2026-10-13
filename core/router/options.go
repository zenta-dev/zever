package router

import (
	"errors"
	"unicode"

	"github.com/zenta-dev/zever/core/log"
)

// Options holds typed configuration for router adapters.
type Options struct {
	// AppName is the application name used by adapters, defaulting to "app" when empty.
	AppName string `json:"app_name" toml:"app_name" yaml:"app_name"`
	// TLSCertFile is the path to the PEM-encoded TLS certificate the
	// generated server presents on its HTTP and gRPC listeners. Empty
	// disables TLS (plaintext). Must be paired with TLSKeyFile.
	TLSCertFile string `json:"tls_cert" toml:"tls_cert" yaml:"tls_cert"`
	// TLSKeyFile is the path to the PEM-encoded TLS private key matching
	// TLSCertFile. Empty disables TLS (plaintext). Must be paired with
	// TLSCertFile.
	TLSKeyFile string `json:"tls_key" toml:"tls_key" yaml:"tls_key"`
	// TLSClientCAFile is the path to a PEM-encoded CA bundle used to
	// verify client certificates (mutual TLS) on the HTTP and gRPC
	// listeners. Empty disables client verification. Requires the
	// TLSCertFile/TLSKeyFile pair: a client CA without a server identity
	// serves nothing to verify against.
	TLSClientCAFile string `json:"tls_client_ca" toml:"tls_client_ca" yaml:"tls_client_ca"`
	// Logger emits skipped-route warnings. Defaults to a no-op logger when nil.
	Logger log.Logger `json:"-" toml:"-" yaml:"-"`
}

// Validate checks options for consistency, joining all violations.
//
// Validate checks Options for logical correctness, joining all violations.
// An empty AppName is valid and lets the adapter default to "app". TLS
// paths are presence-paired only: TLSCertFile requires TLSKeyFile and vice
// versa, and TLSClientCAFile requires both. File existence and PEM validity
// are checked at serve time, not here, so validation stays pure.
func (o Options) Validate() error {
	var errs []error

	if o.AppName != "" {
		if len(o.AppName) > 64 {
			errs = append(errs, InvalidOptionsError{Reason: "app_name must be at most 64 characters"})
		}

		for _, r := range o.AppName {
			if unicode.IsControl(r) {
				errs = append(errs, InvalidOptionsError{Reason: "app_name must not contain control characters"})
				break
			}
		}
	}

	switch {
	case o.TLSCertFile != "" && o.TLSKeyFile == "":
		errs = append(errs, InvalidOptionsError{Reason: "tls_cert requires tls_key: set both or neither"})
	case o.TLSKeyFile != "" && o.TLSCertFile == "":
		errs = append(errs, InvalidOptionsError{Reason: "tls_key requires tls_cert: set both or neither"})
	case o.TLSClientCAFile != "" && (o.TLSCertFile == "" || o.TLSKeyFile == ""):
		errs = append(errs, InvalidOptionsError{Reason: "tls_client_ca requires the tls_cert/tls_key pair: mTLS needs a server identity"})
	}

	return errors.Join(errs...)
}
