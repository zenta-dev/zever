// Package vault provides a secrets.Secrets implementation backed by Vault KVv2 over HTTP.
//
// Security: Addr must use https. The token travels in the X-Vault-Token
// header, so plain http would send it in cleartext to anyone on the path.
// Set AllowInsecure only for test setups (httptest servers speak http
// only); New logs a warning when it is enabled. Round-trips use a client
// with a TLS 1.2 floor and a bounded timeout (DefaultHTTPTimeout).
//
// A token file (TokenFile) must be readable by its owner only (0600 or
// stricter); a file with any group/other permission bit fails closed with
// ErrInsecureTokenFilePerms. Restrict the file to 0600, or set
// AllowInsecureTokenFilePerms to permit broader access (New logs a
// warning when the file is actually permissive).
//
// The mount path is escaped per slash-separated segment before it is
// interpolated into request URLs, so reserved characters cannot shift path
// segments; empty and dot segments are rejected at validation.
package vault
