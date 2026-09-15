package session

import (
	"crypto/rand"
	"encoding/hex"
	"io"
)

// idBytes is the entropy per session ID: 256 bits from crypto/rand,
// hex-encoded to 64 lowercase characters. UUIDv7 is deliberately NOT
// used: its timestamp prefix leaks creation time and its random bits
// are an unsuitable bearer secret. Session IDs are bearer tokens and
// must never be logged or echoed in error messages.
const idBytes = 32

// randReader is the CSPRNG source, swappable in tests to simulate host
// failure. Production always uses crypto/rand.
var randReader = rand.Reader

// NewID generates a fresh opaque session ID: 32 crypto/rand bytes
// hex-encoded (64 chars). It panics only if the OS CSPRNG fails,
// which indicates a broken host, not a caller error.
func NewID() string {
	var b [idBytes]byte
	if _, err := io.ReadFull(randReader, b[:]); err != nil {
		panic("session: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// ValidateID checks a session ID for shape: exactly 64 lowercase hex
// characters as produced by NewID. Failures never echo ID bytes:
// InvalidIDError carries the length for errors.As while Error prints
// the length only.
func ValidateID(id string) error {
	if len(id) != 2*idBytes {
		return &InvalidIDError{IDLen: len(id)}
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return &InvalidIDError{IDLen: len(id)}
			}
		}
	}
	return nil
}
