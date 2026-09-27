// Package postgres provides a db.DB backed by PostgreSQL via pgxpool.
//
// Query placeholders use `?` and are rewritten to PostgreSQL `$n` positional
// parameters. The adapter deliberately does not implement db.Preparer: pgx
// caches prepared statements per connection by default
// (QueryExecModeCacheStatement), so an adapter-level cache would duplicate
// work pgx already does.
//
// Integration tests are gated on the POSTGRES_DSN environment variable and
// skip without a live server.
package postgres
