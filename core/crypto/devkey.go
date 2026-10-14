package crypto

import (
	"os"
	"strings"
)

// DevCryptoKey is the deterministic, public AES-256 key that config.Default()
// ships so zero-infra dev/test runs need no secret setup. It is the base64 of
// the 32-byte ASCII string "01234567890123456789012345678901". It is NOT a
// secret and must never protect production data.
const DevCryptoKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="

// IsDevCryptoKey reports whether key is the deterministic dev/test key that
// config.Default() ships. Production deployments must supply a real key; this
// helper lets them fail closed instead of silently running with a publicly
// known key.
func IsDevCryptoKey(key string) bool {
	return key == DevCryptoKey
}

// devKeyEnvs are the ZEVER_ENV values that mark a non-production
// deployment where the deterministic dev key is acceptable.
var devKeyEnvs = []string{"dev", "test", "development"}

// IsDevEnv reports whether value, a ZEVER_ENV setting, names a
// non-production environment: dev, test or development, after trimming
// surrounding whitespace and ignoring case. Every other value, including
// production and the empty string, is non-dev. It is the single shared
// definition used by DevKeyAllowed and the zever CLI.
func IsDevEnv(value string) bool {
	env := strings.ToLower(strings.TrimSpace(value))
	for _, e := range devKeyEnvs {
		if env == e {
			return true
		}
	}
	return false
}

// DevKeyAllowed reports whether the deterministic dev key may protect
// data: either explicitly via opts.AllowDevKey or implicitly via
// ZEVER_ENV ∈ {dev, test, development} (trimmed, case-insensitive; see
// IsDevEnv). Every other environment — including production and an unset
// ZEVER_ENV — must supply a real key.
func DevKeyAllowed(opts Options) bool {
	return opts.AllowDevKey || IsDevEnv(os.Getenv("ZEVER_ENV"))
}
