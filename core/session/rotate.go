package session

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Rotate replaces the session behind oldID with a fresh one, carrying the
// data forward. It mitigates session fixation after privilege changes
// (notably login): call it once authentication succeeds and switch the
// client to the returned session ID, so a pre-login ID an attacker may know
// can never become authenticated.
//
// The old session must resolve (missing or expired IDs fail with
// ErrNotFound, malformed IDs with ErrInvalidID); no replacement is minted
// in those cases. The fresh session is created with ttl and the old data
// deep-copied onto it; only the data travels, never the ID or timestamps.
// If the old ID cannot be deleted afterwards, Rotate still returns the
// usable fresh session together with the cleanup error: adopt the new ID
// and let the orphaned old session die by expiry. If the replacement write
// itself fails, the fresh session is removed best-effort and only the old
// session (if any) remains. This is a free helper over the Store interface;
// no interface change was needed, so existing backends support it.
func Rotate(ctx context.Context, s Store, oldID string, ttl time.Duration) (Session, error) {
	if s == nil {
		return Session{}, errors.New("session: rotate: nil store")
	}
	if err := ValidateID(oldID); err != nil {
		return Session{}, fmt.Errorf("session: rotate: %w", err)
	}
	old, err := s.Get(ctx, oldID)
	if err != nil {
		return Session{}, fmt.Errorf("session: rotate: %w", err)
	}
	fresh, err := s.Create(ctx, ttl)
	if err != nil {
		return Session{}, fmt.Errorf("session: rotate: %w", err)
	}
	fresh.Data = old.Clone().Data
	if err := s.Save(ctx, fresh); err != nil {
		_ = s.Delete(ctx, fresh.ID)
		return Session{}, fmt.Errorf("session: rotate: %w", err)
	}
	if err := s.Delete(ctx, oldID); err != nil {
		return fresh, fmt.Errorf("session: rotate: delete old: %w", err)
	}
	return fresh, nil
}
