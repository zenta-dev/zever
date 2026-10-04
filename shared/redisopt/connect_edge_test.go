package redisopt

import (
	"strings"
	"testing"
)

// TestValidatePrefix_byteLength verifies the limit is measured in bytes.
func TestValidatePrefix_byteLength(t *testing.T) {
	t.Parallel()

	// 22 three-byte runes = 66 bytes > 64, though only 22 characters.
	tooLong := strings.Repeat("世", 22)
	if err := ValidatePrefix(tooLong); err == nil {
		t.Errorf("ValidatePrefix(%d-byte prefix) = nil, want length error", len(tooLong))
	}
}
