// Package header resolves tenants from a request header with subdomain fallback.
//
// Trust boundary: this adapter trusts the header/subdomain value it is
// given as-is, with no verification that the caller is entitled to the
// tenant it names. Deploy it only behind a reverse proxy or gateway that
// authenticates the caller and strips or overwrites the tenant header
// before the request reaches this code; never expose it directly to
// untrusted clients, or any client can select an arbitrary tenant.
package header
