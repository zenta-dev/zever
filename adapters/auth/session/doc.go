// Package session provides a session-store-backed auth.Auth implementation.
//
// Tokens are opaque session IDs minted by the store; subject, claims, and
// expiry travel inside the session Data envelope (keys "sub", "custom",
// "exp" as unix seconds). Unknown or store-expired sessions verify as
// auth.ErrInvalidToken with no revoked-vs-unknown distinction. Envelope
// expiry carries a 1-minute leeway for clock skew before reporting
// auth.ErrTokenExpired. Revocation is idempotent: deleting an unknown ID
// is nil per the session.Store contract.
package session
