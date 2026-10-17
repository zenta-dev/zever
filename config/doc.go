// Package config resolves layered service configuration from safe defaults
// and environment variables, with redaction as the sole display path for
// option maps.
//
// Layering (later layers win):
//
//	Default: zero-infrastructure adapters plus deterministic dev secrets
//	so tests run without manual setup. Dev secrets are not for production;
//	production must supply real secrets via environment.
//
//	Env: variables carry no prefix: DB_ADAPTER selects the db adapter,
//	DB_MAXCONNS sets a db field. Files (.env, .env.local) feed the same
//	layer beneath the real process environment; see LoadEnv and FromEnv.
//
// A service is one zever package's Adapter plus Options: the adapter
// names the backend implementation (for example "sqlite" or "memory")
// and Options carries that backend's typed settings. Named instances add
// a middle segment (DB__REPORTING__DSN); see ApplyEnv and InstanceNames.
//
// Display rule: raw option maps may hold secrets in plain strings.
// Never log them directly. Redact (and the redacted service views built
// on it) is the only path for logging or debugging output.
package config
