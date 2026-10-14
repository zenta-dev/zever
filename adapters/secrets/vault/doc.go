// Package vault provides a secrets.Secrets implementation backed by Vault KVv2 over HTTP.
//
// Security: Addr must use https. The token travels in the X-Vault-Token
// header, so plain http would send it in cleartext to anyone on the path.
// Set AllowInsecure only for test setups (httptest servers speak http
// only); New logs a warning when it is enabled. Round-trips use a client
// with a TLS 1.2 floor and a bounded timeout (DefaultHTTPTimeout).
package vault
