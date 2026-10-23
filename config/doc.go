// Package config resolves layered service configuration from safe defaults
// and environment variables, with redaction as the sole display path for
// option maps.
//
// Layering (later layers win):
//
//	Default: options-only zero-infrastructure values so tests run without
//	manual setup. Dev secrets are deterministic and must not ship to
//	production; production must supply real secrets via environment.
//
//	Env: variables carry no prefix: DB_MAX_CONNS sets a db field. Files
//	(.env, .env.local) feed the same layer beneath the real process
//	environment; see LoadEnv and FromEnv.
//
// Backend selection is compile-time, not configuration: adapters are wired
// in the app's wiring module with container.Provide plus direct imports. The environment
// only tunes options; any *_ADAPTER key fails resolution, and a Config
// carries no adapter names. Named instances add a middle segment
// (DB__REPORTING__DSN); see ApplyEnv and InstanceNames.
//
// Display rule: raw option maps may hold secrets in plain strings.
// Never log them directly. Redact (and the redacted service views built
// on it) is the only path for logging or debugging output.
package config
