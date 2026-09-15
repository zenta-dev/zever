package flag

// maxKeyLen is the maximum accepted flag key length in bytes.
const maxKeyLen = 256

// ValidateKey checks a flag key for shape.
// Keys must be 1..256 bytes and must not contain control characters
// (including CR/LF) or DEL. Failures return *InvalidKeyError carrying
// the key length only; the key itself is never echoed.
func ValidateKey(key string) error {
	if len(key) < 1 || len(key) > maxKeyLen {
		return &InvalidKeyError{KeyLen: len(key)}
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 || key[i] == 0x7F {
			return &InvalidKeyError{KeyLen: len(key)}
		}
	}
	return nil
}
