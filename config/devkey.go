package config

// devCryptoKey is the deterministic, public AES-256 key Default() ships so
// zero-infra dev/test runs need no secret setup. It is the base64 of the
// 32-byte ASCII string "01234567890123456789012345678901". It is NOT a
// secret and must never protect production data.
const devCryptoKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="

// IsDevCryptoKey reports whether key is the deterministic dev/test key that
// Default() ships. Production deployments must supply a real key; this helper
// lets them fail closed (for example when ZEVER_CRYPTO_REQUIRE_REAL_KEY is
// set) instead of silently running with a publicly known key.
func IsDevCryptoKey(key string) bool {
	return key == devCryptoKey
}
