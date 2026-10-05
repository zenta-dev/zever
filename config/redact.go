package config

import (
	"strings"
)

// RedactedValue replaces secret values in redacted option maps.
const RedactedValue = "[REDACTED]"

// foldASCII lowercases one ASCII byte, passing every other byte through
// unchanged so callers can gate on isASCII before folding.
func foldASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// isASCII reports whether s holds only ASCII bytes.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// asciiEqualFold reports whether s and t are equal under ASCII case
// folding, without allocation.
func asciiEqualFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if foldASCII(s[i]) != foldASCII(t[i]) {
			return false
		}
	}
	return true
}

// asciiContainsFold reports whether s contains substr under ASCII case
// folding, without allocation.
func asciiContainsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	first := foldASCII(substr[0])
	for i := 0; i+len(substr) <= len(s); i++ {
		if foldASCII(s[i]) == first && asciiEqualFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// isSensitiveKey reports whether a config key likely holds a secret.
// Substring rules catch variants such as "secret_key", "webhook_secret",
// or "bearer_token". Exact rules cover short names where substring
// matching would over-fire (for example "auth" must not redact "author").
//
// The common case — an already-lowercase ASCII key, as produced by
// JSON-tagged option maps — matches without allocation. Mixed-case ASCII
// keys fold in place; only non-ASCII keys fall back to strings.ToLower.
func isSensitiveKey(k string) bool {
	if !isASCII(k) {
		return isSensitiveKeyLower(strings.ToLower(k))
	}

	// Fast path: key already lowercase.
	if strings.Contains(k, "password") || strings.Contains(k, "secret") ||
		strings.Contains(k, "token") || strings.Contains(k, "bearer") ||
		strings.Contains(k, "credential") {
		return true
	}
	switch k {
	case "dsn", "api_key", "key", "sign_key", "encrypt_key", "hmac_key",
		"access_key", "secret_key", "private_key", "client_secret",
		"auth", "session", "cookie", "apikey", "passwd", "pwd":
		return true
	}

	// Slow path: case-insensitive match without allocation.
	if asciiContainsFold(k, "password") || asciiContainsFold(k, "secret") ||
		asciiContainsFold(k, "token") || asciiContainsFold(k, "bearer") ||
		asciiContainsFold(k, "credential") {
		return true
	}
	switch {
	case asciiEqualFold(k, "dsn"), asciiEqualFold(k, "api_key"),
		asciiEqualFold(k, "key"), asciiEqualFold(k, "sign_key"),
		asciiEqualFold(k, "encrypt_key"), asciiEqualFold(k, "hmac_key"),
		asciiEqualFold(k, "access_key"), asciiEqualFold(k, "secret_key"),
		asciiEqualFold(k, "private_key"), asciiEqualFold(k, "client_secret"),
		asciiEqualFold(k, "auth"), asciiEqualFold(k, "session"),
		asciiEqualFold(k, "cookie"), asciiEqualFold(k, "apikey"),
		asciiEqualFold(k, "passwd"), asciiEqualFold(k, "pwd"):
		return true
	}

	return false
}

// isSensitiveKeyLower applies the substring and exact rules to an
// already-lowered key. It backs the non-ASCII fallback in isSensitiveKey.
func isSensitiveKeyLower(lower string) bool {
	// Substring rules, ported verbatim from the reference implementation:
	// password, secret, token, bearer, credential.
	if strings.Contains(lower, "password") || strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") || strings.Contains(lower, "bearer") ||
		strings.Contains(lower, "credential") {
		return true
	}

	// Exact rules, ported verbatim from the reference implementation:
	// dsn, api_key, key, sign_key, encrypt_key, hmac_key, access_key,
	// secret_key, private_key, client_secret.
	switch lower {
	case "dsn", "api_key", "key", "sign_key", "encrypt_key", "hmac_key",
		"access_key", "secret_key", "private_key", "client_secret":
		return true
	}

	// Exact-rule extensions, each backed by a cited source. They stay
	// exact (not substring) so common words cannot over-fire:
	//   auth, session, apikey — sensitive query params in hermes-agent's
	//   redaction denylist; session tokens must never reach logs per the
	//   OWASP logging cheat sheet.
	//   cookie — carries session tokens (OWASP: mask session tokens).
	//   passwd, pwd — generic password-assignment patterns in
	//   secret-scanning rule sets (password|passwd|pwd|pass).
	switch lower {
	case "auth", "session", "cookie", "apikey", "passwd", "pwd":
		return true
	}

	return false
}

// redactMap replaces secret values in m in place, recursing into nested
// maps and slices. Empty strings are left alone so unset fields are not
// confused with set-but-hidden secrets. Non-string secrets are still
// redacted.
func redactMap(m map[string]any) {
	for k, v := range m {
		if isSensitiveKey(k) {
			switch val := v.(type) {
			case string:
				if val != "" {
					m[k] = RedactedValue
				}
			default:
				if v != nil {
					m[k] = RedactedValue
				}
			}

			continue
		}

		switch val := v.(type) {
		case map[string]any:
			redactMap(val)
		case []map[string]any:
			for _, nested := range val {
				redactMap(nested)
			}
		case []any:
			redactSlice(val)
		case []string:
			redactStringSlice(val)
		}
	}
}

// redactSlice redacts secrets inside slice elements in place,
// recursing into nested maps and slices.
func redactSlice(s []any) {
	for _, elem := range s {
		switch v := elem.(type) {
		case map[string]any:
			redactMap(v)
		case []map[string]any:
			for _, nested := range v {
				redactMap(nested)
			}
		case []any:
			redactSlice(v)
		case []string:
			redactStringSlice(v)
		}
	}
}

// redactStringSlice redacts "key: value"-shaped entries in place, such as an
// HTTP header list (["Authorization: Bearer xxx", "Content-Type: json"])
// nested under a key isSensitiveKey does not itself match (e.g. "headers").
// Entries with no ": " separator, or whose key half is not sensitive, are
// left untouched -- this only closes the header-list-shaped case; a bare
// secret string with no key prefix is out of scope for key-based redaction.
func redactStringSlice(s []string) {
	for i, entry := range s {
		key, val, ok := strings.Cut(entry, ": ")
		if !ok || val == "" {
			continue
		}

		if isSensitiveKey(strings.TrimSpace(key)) {
			s[i] = key + ": " + RedactedValue
		}
	}
}

// Redact returns a deep copy of m with secret values replaced by
// RedactedValue. The caller's map is never mutated.
//
// Copy and redaction run in a single recursive pass: values under
// sensitive keys are replaced instead of copied, so no allocation is
// spent on subtrees that are about to be dropped. A hand-rolled pass is
// used instead of a JSON round-trip on purpose: JSON would coerce
// numbers to float64, drop non-JSON values such as time.Duration, and
// fail on values Marshal cannot represent.
func Redact(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	return redactCopyMap(m)
}

// redactCopyMap deep-copies m, replacing secret values with RedactedValue
// in the same pass.
func redactCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = redactCopyValue(v, isSensitiveKey(k))
	}

	return out
}

// redactCopyValue deep-copies v. When sensitive is true the value is a
// secret: set values collapse to RedactedValue while empty strings and
// nil stay untouched, so unset fields are not confused with
// set-but-hidden secrets.
func redactCopyValue(v any, sensitive bool) any {
	if sensitive {
		switch val := v.(type) {
		case string:
			if val != "" {
				return RedactedValue
			}

			return val
		default:
			if v != nil {
				return RedactedValue
			}

			return v
		}
	}

	switch val := v.(type) {
	case map[string]any:
		return redactCopyMap(val)
	case []map[string]any:
		out := make([]map[string]any, len(val))
		for i, nested := range val {
			out[i] = redactCopyMap(nested)
		}

		return out
	case []any:
		out := make([]any, len(val))
		for i, elem := range val {
			out[i] = redactCopyValue(elem, false)
		}

		return out
	case []string:
		out := make([]string, len(val))
		copy(out, val)
		redactStringSlice(out)

		return out
	default:
		return v
	}
}
