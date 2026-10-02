// Package oidc provides an OpenID Connect authentication adapter.
//
// The adapter is verify-only by design: Issue and Revoke always fail with
// auth.ErrNotSupported. Token issuance and revocation are the identity
// provider's job; this package only validates ID tokens the IdP minted.
//
// Trust is delegated to the IdP via OIDC discovery. The adapter holds no
// local signing secrets: verification keys come from the provider's JWKS
// endpoint discovered at construction time.
//
// Revocation is the IdP's job too: this adapter keeps no revocation list,
// so a compromised ID token verifies until it expires. Keep IdP lifetimes
// short and revoke at the provider (session/end-session, token revocation
// endpoint); a locally "revoked" token cannot be enforced here.
package oidc
