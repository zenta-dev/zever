package idempotency

import (
	"slices"
	"unicode"
	"unicode/utf8"
)

// ValidateKey checks an idempotency key for length and content.
// Valid keys are 1..MaxKeyLen bytes with no control characters
// (including \r, \n) and no DEL. Keys may carry sensitive material,
// so failures never echo key bytes: InvalidKeyError carries the length
// for errors.As while Error prints the length only.
func ValidateKey(key string) error {
	if len(key) == 0 || len(key) > MaxKeyLen {
		return &InvalidKeyError{KeyLen: len(key)}
	}

	// No raw-byte walk is needed: UTF-8 is self-synchronizing, so bytes
	// <0x20 and DEL never hide inside multi-byte sequences — any such byte
	// decodes standalone in range and trips the check above.
	for _, r := range key {
		if r == utf8.RuneError {
			continue
		}
		if r < 0x20 || r == 0x7f || unicode.IsControl(r) {
			return &InvalidKeyError{KeyLen: len(key)}
		}
	}

	return nil
}

// FingerprintMatches reports whether stored and incoming fingerprints match.
// Both empty (nil or zero-length) counts as a match for dirty
// back-compat; a record with a fingerprint versus an empty incoming
// fingerprint is a strict mismatch.
func FingerprintMatches(stored, incoming []byte) bool {
	if len(stored) == 0 && len(incoming) == 0 {
		return true
	}

	return slices.Equal(stored, incoming)
}
