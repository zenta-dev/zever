package config

import (
	"strings"
)

// RedactedValue replaces secret values in redacted option maps.
const RedactedValue = "[REDACTED]"

// isSensitiveKey reports whether a config key likely holds a secret.
// Substring rules catch variants such as "secret_key", "webhook_secret",
// or "bearer_token". Exact rules cover short names where substring
// matching would over-fire (for example "auth" must not redact "author").
func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
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
		}
	}
}

// deepCopyValue clones maps and slices recursively. Scalars (strings,
// numbers, bools, and exotic values such as time.Duration) are immutable
// or caller-owned, so they pass through by reference.
// deepCopyMap copies a string-keyed map recursively.
func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, elem := range m {
		out[k] = deepCopyValue(elem)
	}

	return out
}

func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, elem := range val {
			out[k] = deepCopyValue(elem)
		}

		return out
	case []map[string]any:
		out := make([]map[string]any, len(val))
		for i, nested := range val {
			out[i] = deepCopyMap(nested)
		}

		return out
	case []any:
		out := make([]any, len(val))
		for i, elem := range val {
			out[i] = deepCopyValue(elem)
		}

		return out
	default:
		return v
	}
}

// Redact returns a deep copy of m with secret values replaced by
// RedactedValue. The caller's map is never mutated.
//
// A hand-rolled recursive copy is used instead of a JSON round-trip on
// purpose: JSON would coerce numbers to float64, drop non-JSON values
// such as time.Duration, and fail on values Marshal cannot represent.
func Redact(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}

	redactMap(out)

	return out
}
