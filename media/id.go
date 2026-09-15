package media

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// ValidID reports whether s is a safe media asset identifier.
// Identifiers may contain ASCII letters, digits, hyphens, and underscores.
// Empty, ".", "..", and values containing ".." are rejected.
func ValidID(s string) bool {
	switch s {
	case "", ".", "..":
		return false
	}

	if strings.Contains(s, "..") {
		return false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}

		return false
	}

	return true
}

// ValidHexID reports whether s is a 32-character hex identifier.
func ValidHexID(s string) bool {
	if len(s) != 32 {
		return false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}

		return false
	}

	return true
}

// GenerateID generates a random 128-bit hex media identifier.
func GenerateID() (string, error) {
	return GenerateIDWithReader(rand.Reader)
}

// GenerateIDWithReader generates a random 128-bit hex identifier from r.
func GenerateIDWithReader(r io.Reader) (string, error) {
	var b [16]byte

	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", fmt.Errorf("media: generate id: %w", err)
	}

	return hex.EncodeToString(b[:]), nil
}
