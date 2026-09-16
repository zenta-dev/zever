// Package config loads layered service configuration with safe defaults,
// strict file decoding, best-effort environment matching, and redaction
// as the sole display path for option maps.
//
// Layering (later layers win):
//
//	Default: zero-infrastructure adapters plus deterministic dev secrets
//	so tests run without manual setup. Dev secrets are not for production;
//	production must supply real secrets via file or environment.
//
//	File: YAML or JSON decoded strictly. Unknown services and
//	unknown fields are errors, so typos fail fast instead of being
//	silently ignored.
//
//	Environment: variables carry no prefix. Matching is best-effort:
//	unknown variables are ignored, never errors, so unrelated process
//	environment cannot break startup.
//
// A service is one zever package's Adapter plus Options: the adapter
// names the backend implementation (for example "sqlite" or "memory")
// and Options carries that backend's typed settings.
//
// Display rule: raw option maps may hold secrets in plain strings.
// Never log them directly. Redact (and the redacted service views built
// on it) is the only path for logging or debugging output.
package config
