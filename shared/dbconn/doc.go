// Package dbconn shares the DB-routing predicates every DB-backed adapter
// needs: DSN sniffing (postgres URL vs sqlite path), single-string DSN to
// pool-options mapping, DDL-interpolated table-name validation, and
// random-per-driver lease-owner minting.
//
// Deliberately NOT shared: the New/Open bodies that resolve an already
// split coredb.Options (DSN vs Path fields). There a set DSN field means
// postgres by construction, so adapters test it with a plain non-empty
// check; folding that through IsPostgresDSN would silently reroute a
// file path set in the DSN field to sqlite. Same reason the sqlite
// ":memory:" defaulting stays per adapter.
package dbconn
