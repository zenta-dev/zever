// Package smtp provides an SMTP mailer.Mailer.
//
// Two transport modes exist: implicit TLS (full TLS from connect) and
// STARTTLS (mandatory upgrade of a plaintext connection; the upgrade is
// refused when the server does not advertise it, blocking STRIPTLS
// downgrade). Plaintext (EncryptionNone) carries no credentials.
package smtp
