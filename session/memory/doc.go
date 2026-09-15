// Package memory provides an in-memory session.Store.
//
// The store keeps sessions in a mutex-guarded map with fixed lifetimes:
// expiry is absolute per ExpiresAt and never slides on read. Expired
// sessions are treated exactly like missing ones (ErrNotFound) and are
// removed lazily on access plus by a background sweeper.
//
// Save semantics: saving an existing session replaces Data atomically
// (last-write-wins) and touches UpdatedAt but keeps the original
// ExpiresAt (never extended). Saving an
// unknown ID upserts with ExpiresAt = now + store-default TTL, ignoring
// any caller-provided ExpiresAt (fail-closed against immortal sessions).
//
// Copies: Data is deep-copied in both directions via Session.Clone, so
// callers never alias stored state. Values are caller-owned; nested
// maps, slices, and byte slices are copied, other values are shared by
// reference (scalar/small-map expectation).
package memory
