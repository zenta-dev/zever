// Package static provides a file-backed feature flag driver.
//
// Flags load once from a JSON object file at construction. When Reload
// is enabled the driver re-reads the file on each lookup, gated by
// modtime and content hash, keeping the last-good set on any failure.
// An empty path yields an empty flag set for local development.
package static
