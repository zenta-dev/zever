// Package mailer sends mail with swappable adapters.
//
// It validates addresses and options, enforces size limits, and delivers Mail
// through log or SMTP backends. It does not render templates or manage
// mailing lists, only validated delivery.
//
// Type safety: Mailer interface with typed Options plus Adapter enum plus
// Factory. Encryption enum plus Mail, Address, Attachment, Sender, and
// NewMail with Clone validation. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests, with the log adapter writing instead of
// sending. Config file plus env MAILER_ADAPTER (no prefix, e.g.
// MAILER_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Mailer(). Lazy per-service singleton,
// retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Send takes ctx plus the
// message, Close releases resources and takes no ctx. Close is idempotent on
// the log adapter. Only resolved services close.
//
// Errors: sentinel errors, errors.Is compatible, prefixed mailer:. Name
// service and field only in errors, with DuplicateError, UnknownAdapterError,
// InvalidAdapterError, InvalidOptionsError, and InvalidAddressError carrying
// the adapter, reason, field, or value.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// STARTTLS is the default encryption and EncryptionNone must not carry
// credentials. Address validation rejects header injection and control
// characters. Passwords are never logged.
//
// Performance: operation timeout defaults to 30s. Message size defaults to a
// 10MB cap with ErrMessageTooLarge beyond it. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init
// wiring.
//
// Example: see ExampleOpen in example_test.go.
package mailer
