package mailer

import (
	"strings"
	"time"
)

// Encryption identifies the SMTP transport encryption mode.
type Encryption string

const (
	// EncryptionSTARTTLS upgrades a plaintext connection via STARTTLS.
	EncryptionSTARTTLS Encryption = "starttls"
	// EncryptionImplicitTLS uses implicit TLS from connection start.
	EncryptionImplicitTLS Encryption = "implicit"
	// EncryptionNone disables transport encryption.
	EncryptionNone Encryption = "none"
)

const (
	// DefaultTimeout is the default SMTP operation timeout applied by adapters.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxMessageSize is the default maximum message size in bytes.
	DefaultMaxMessageSize = 10 << 20
)

// Options configures mailer construction.
type Options struct {
	// Host is the SMTP server hostname.
	Host string
	// Port is the SMTP server port (1-65535).
	Port int
	// Username is the SMTP auth username.
	Username string
	// Password is the SMTP auth password.
	Password string
	// Encryption is the transport encryption mode. Empty means STARTTLS.
	Encryption Encryption
	// Timeout is the operation timeout. Zero means the default.
	Timeout time.Duration
	// MaxMessageSize is the maximum message size in bytes. Zero means the default.
	MaxMessageSize int
}

// Validate checks options for consistency.
// Zero Timeout and MaxMessageSize mean "apply defaults" and are valid;
// only negative values fail.
func (o Options) Validate() error {
	if strings.TrimSpace(o.Host) == "" {
		return &InvalidOptionsError{Reason: "host must be non-empty"}
	}
	if strings.Contains(o.Host, "://") {
		return &InvalidOptionsError{Reason: "host must not contain scheme"}
	}

	if o.Port < 1 || o.Port > 65535 {
		return &InvalidOptionsError{Reason: "port must be 1-65535"}
	}

	if (o.Username == "") != (o.Password == "") {
		return &InvalidOptionsError{Reason: "username and password must be set together"}
	}

	switch o.Encryption {
	case "", EncryptionSTARTTLS, EncryptionImplicitTLS, EncryptionNone:
	default:
		return &InvalidOptionsError{Reason: "encryption must be starttls, implicit, or none"}
	}

	if o.Encryption == EncryptionNone && (o.Username != "" || o.Password != "") {
		return &InvalidOptionsError{Reason: "encryption none must not carry credentials"}
	}

	if o.Timeout < 0 {
		return &InvalidOptionsError{Reason: "timeout must be >= 0"}
	}

	if o.MaxMessageSize < 0 {
		return &InvalidOptionsError{Reason: "max message size must be >= 0"}
	}

	return nil
}

// encryption returns the effective encryption mode, defaulting to STARTTLS.
func (o Options) encryption() Encryption {
	if o.Encryption == "" {
		return EncryptionSTARTTLS
	}
	return o.Encryption
}

// timeout returns the effective timeout, defaulting to DefaultTimeout.
func (o Options) timeout() time.Duration {
	if o.Timeout == 0 {
		return DefaultTimeout
	}
	return o.Timeout
}

// maxMessageSize returns the effective size limit, defaulting to DefaultMaxMessageSize.
func (o Options) maxMessageSize() int {
	if o.MaxMessageSize == 0 {
		return DefaultMaxMessageSize
	}
	return o.MaxMessageSize
}
