// Package sqlite provides a durable webhook adapter.
//
// Registrations persist in SQLite while delivery stays synchronous: the
// adapter composes the http adapter as its delivery engine and delegates
// Register validation plus all delivery to it, adding only durable storage.
//
// Secrets are stored in plaintext in the SQLite database. File permissions
// and safe handling of the database file are the operator's responsibility;
// this package introduces no additional encryption.
package sqlite
