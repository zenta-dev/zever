// Package revocation defines the pluggable store the jwt adapter uses to
// track revoked tokens, so revocation state can be shared and made durable
// across instances instead of living only in one process's memory.
//
// Entries are keyed by the JWT jti (JWT ID) claim, never by the raw token
// string: the raw token is a bearer secret and must not be replicated into a
// second store, and jti keys are small and stable regardless of how the
// token itself is encoded. A token that carries no jti cannot be revoked
// individually through a Store; callers must reject revocation attempts for
// such tokens (the jwt adapter does exactly this, see auth/jwt.Adapter).
package revocation

import (
	"context"
	"errors"
	"time"
)

// ErrClosed is returned by Store methods called after Close.
var ErrClosed = errors.New("revocation: store is closed")

// ErrNoJTI is returned when a caller attempts to revoke a token that carries
// no jti claim. There is no raw-token fallback key: hashing the raw token
// would still require distributing the token bytes to the caller computing
// the hash, and a truncated hash would reintroduce collision risk. Tokens
// that must be revocable should always be issued with a jti.
var ErrNoJTI = errors.New("revocation: token has no jti claim")

// Store tracks revoked token identifiers (JWT jti claims) until their
// natural expiry. Implementations must be safe for concurrent use.
type Store interface {
	// Revoke marks jti as revoked until until. It is idempotent: revoking
	// an already-revoked jti returns nil. Implementations reject an empty
	// jti with an error.
	Revoke(ctx context.Context, jti string, until time.Time) error

	// IsRevoked reports whether jti is currently revoked. An unknown or
	// expired jti reports false, not an error.
	IsRevoked(ctx context.Context, jti string) (bool, error)

	// Close releases resources held by the store. It is idempotent.
	Close() error
}
