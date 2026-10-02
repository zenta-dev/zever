// Package session provides a session-store-backed auth.Auth implementation.
//
// Tokens are opaque session IDs minted by the store; subject, claims, and
// expiry travel inside the session Data envelope (keys "sub", "custom",
// "exp" as unix seconds). Unknown or store-expired sessions verify as
// auth.ErrInvalidToken with no revoked-vs-unknown distinction. Envelope
// expiry carries a 1-minute leeway for clock skew before reporting
// auth.ErrTokenExpired. Revocation is idempotent: deleting an unknown ID
// is nil per the session.Store contract.
//
// Indistinguishability is deliberate: Verify maps a store miss (never
// issued, already revoked via Delete, or reaped by store TTL) and a corrupt
// envelope to the same auth.ErrInvalidToken, so callers cannot probe which
// tokens exist. Only a live, well-formed, unexpired session verifies; treat
// any InvalidToken as "no such session" and never branch on finer causes.
package session
