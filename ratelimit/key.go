package ratelimit

// ValidateKey checks a rate-limit key for validity.
// Keys must be 1..MaxKeyLen bytes and must not contain control characters,
// CR, LF, or DEL. The returned error never echoes the key itself.
func ValidateKey(key string) error {
	n := len(key)
	if n == 0 || n > MaxKeyLen {
		return &InvalidKeyError{KeyLen: n}
	}

	for i := 0; i < n; i++ {
		c := key[i]
		if c < 0x20 || c == 0x7f {
			return &InvalidKeyError{KeyLen: n}
		}
	}

	return nil
}
