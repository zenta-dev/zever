package redis_test

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashID mirrors the adapter's key derivation (hex SHA-256 of the session
// ID) so black-box tests can address the raw Redis key.
func hashID(id string) string {
	sum := sha256.Sum256([]byte(id))

	return hex.EncodeToString(sum[:])
}
