package config

import (
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/zenta-dev/zever/shared/codec"
)

// RedactedValue replaces secret values in redacted option maps.
const RedactedValue = "[REDACTED]"

// Secret is a string option type whose values are always redacted from
// displayed configuration, regardless of the field's key name. Option
// structs that cannot import config (core batteries) mark secret fields
// with the zever:"secret" struct tag instead; the redactor consults both.
type Secret string

// secretType identifies config.Secret fields during the redactor's
// reflection pass over typed options structs.
var secretType = reflect.TypeOf(Secret(""))

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
// redacted. String values under non-sensitive keys that carry URL userinfo
// (for example a DSN under "database_url") have the credentials scrubbed.
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
		case string:
			if scrubbed := scrubValue(k, val); scrubbed != val {
				m[k] = scrubbed
			}
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
			scrubStringSlice(k, val)
		}
	}
}

// scrubStringSlice scrubs URL/DSN credentials from each entry in place; key
// is the name of the slice's owning field.
func scrubStringSlice(key string, s []string) {
	for i, entry := range s {
		s[i] = scrubValue(key, entry)
	}
}

// redactSlice redacts secrets inside slice elements in place,
// recursing into nested maps and slices.
func redactSlice(s []any) {
	for i, elem := range s {
		switch v := elem.(type) {
		case string:
			s[i] = scrubURLUserinfo(v)
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
		sensitive := isSensitiveKey(k)
		if !sensitive {
			switch val := v.(type) {
			case string:
				out[k] = scrubValue(k, val)

				continue
			case []string:
				cp := make([]string, len(val))
				copy(cp, val)
				redactStringSlice(cp)
				scrubStringSlice(k, cp)
				out[k] = cp

				continue
			}
		}
		out[k] = redactCopyValue(v, sensitive)
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
	case string:
		return scrubURLUserinfo(val)
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
		return redactTypedValue(v)
	}
}

// redactSecretFields redacts values in m whose corresponding field in the
// typed options struct opts is marked secret, either by the zever:"secret"
// struct tag or by the config.Secret type. Key-name denylisting already ran
// in redactMap; this pass catches oddly named secret fields the denylist
// cannot know about.
//
// The walk is type-driven and covers every shape a secret can take: plain
// fields, pointers, slices, arrays, maps (string keys), nested structs
// (named or embedded, flattened like encoding/json), and interface-typed
// values, whose dynamic type is read from opts itself. A secret tag or
// Secret type applies to everything beneath it, so a tagged []string or
// map[string]string has every element redacted. Nil pointers, maps, and
// interfaces are tolerated; recursion is bounded by the finite map m, so
// cyclic Go values cannot loop.
func redactSecretFields(opts any, m map[string]any) {
	if opts == nil || m == nil {
		return
	}

	v := reflect.ValueOf(opts)
	walkSecret(v.Type(), v, m, false)
}

// redactNode returns RedactedValue for a set node and the node itself for
// an unset one: empty strings and nil stay untouched so unset fields are
// not confused with set-but-hidden secrets.
func redactNode(node any) any {
	switch val := node.(type) {
	case nil:
		return nil
	case string:
		if val == "" {
			return val
		}
	}

	return RedactedValue
}

// walkSecret redacts node, the JSON-decoded form of a value of type t.
// v is the live value (possibly invalid when only the type is known) and is
// consulted solely to resolve interface-typed values. secret marks that an
// ancestor field was tagged or typed secret. It returns the node to store
// in place of the input; maps and slices are also edited in place.
func walkSecret(t reflect.Type, v reflect.Value, node any, secret bool) any {
	if node == nil {
		return nil
	}

	for {
		if t.Kind() == reflect.Interface {
			if !v.IsValid() || v.IsNil() {
				// Dynamic type unknown: only a secret-flagged ancestor
				// needs handling.
				if secret {
					return redactAll(node)
				}

				return node
			}
			v = v.Elem()
			t = v.Type()

			continue
		}
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
			if v.IsValid() {
				if v.IsNil() {
					v = reflect.Value{}
				} else {
					v = v.Elem()
				}
			}

			continue
		}

		break
	}

	if t == secretType {
		secret = true
	}

	switch n := node.(type) {
	case map[string]any:
		switch t.Kind() { //nolint:exhaustive // only container kinds matter
		case reflect.Struct:
			walkSecretStruct(t, v, n, secret)
		case reflect.Map:
			for k, child := range n {
				var ev reflect.Value
				if v.IsValid() && t.Key().Kind() == reflect.String {
					ev = v.MapIndex(reflect.ValueOf(k).Convert(t.Key()))
				}
				n[k] = walkSecret(t.Elem(), ev, child, secret)
			}
		default:
			if secret {
				return redactAll(n)
			}
		}

		return n
	case []any:
		switch t.Kind() { //nolint:exhaustive // only container kinds matter
		case reflect.Slice, reflect.Array:
			for i, child := range n {
				var ev reflect.Value
				if v.IsValid() && i < v.Len() {
					ev = v.Index(i)
				}
				n[i] = walkSecret(t.Elem(), ev, child, secret)
			}
		default:
			if secret {
				return redactAll(n)
			}
		}

		return n
	default:
		if secret {
			return redactNode(node)
		}

		return node
	}
}

// walkSecretStruct redacts the fields of struct type t in m. Anonymous
// (embedded) struct fields without a JSON name are flattened into the same
// map, matching encoding/json and serviceToMap; named struct fields recurse
// into their own nested value.
func walkSecretStruct(t reflect.Type, v reflect.Value, m map[string]any, secret bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		var fv reflect.Value
		if v.IsValid() {
			fv = v.Field(i)
		}

		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}

		fieldSecret := secret || f.Tag.Get("zever") == "secret"

		if f.Anonymous && name == "" {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				fval := fv
				for fval.IsValid() && fval.Kind() == reflect.Pointer {
					if fval.IsNil() {
						fval = reflect.Value{}
					} else {
						fval = fval.Elem()
					}
				}
				walkSecretStruct(ft, fval, m, fieldSecret)

				continue
			}
		}

		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}

		child, ok := m[name]
		if !ok {
			continue
		}
		m[name] = walkSecret(f.Type, fv, child, fieldSecret)
	}
}

// redactAll redacts every leaf beneath node, used when a secret marker
// applies to a value whose Go type is not known (an untyped interface).
func redactAll(node any) any {
	switch n := node.(type) {
	case map[string]any:
		for k, child := range n {
			n[k] = redactAll(child)
		}

		return n
	case []any:
		for i, child := range n {
			n[i] = redactAll(child)
		}

		return n
	default:
		return redactNode(node)
	}
}

// typeMayHoldSecret reports whether values of type t could carry a secret
// by type: config.Secret, a zever:"secret" tagged field, or an interface
// whose dynamic content is unknown. seen guards recursive types.
func typeMayHoldSecret(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == secretType {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true

	switch t.Kind() { //nolint:exhaustive // only container kinds matter
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return typeMayHoldSecret(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Tag.Get("zever") == "secret" || typeMayHoldSecret(f.Type, seen) {
				return true
			}
		}
	}

	return false
}

// redactTypedValue handles a Go-typed (non-JSON-shaped) value met inside a
// map passed to Redact: a Secret, *Secret, []Secret, map[string]Secret,
// struct with tagged fields, and so on. Such values are converted to their
// JSON shape and redacted by type. Types that cannot carry a secret are
// returned unchanged so durations and other non-JSON values survive.
func redactTypedValue(v any) any {
	rv := reflect.ValueOf(v)
	switch rv.Kind() { //nolint:exhaustive // only container kinds matter
	case reflect.String, reflect.Struct, reflect.Pointer, reflect.Slice,
		reflect.Array, reflect.Map:
	default:
		return v
	}
	if !typeMayHoldSecret(rv.Type(), map[reflect.Type]bool{}) {
		return v
	}
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return v
	}

	b, err := codec.Marshal(v)
	if err != nil {
		return RedactedValue
	}
	var node any
	if err := codec.Unmarshal(b, &node); err != nil {
		return RedactedValue
	}

	return walkSecret(rv.Type(), rv, node, false)
}

// dsnPairRE matches keyword-style credentials such as "password=hunter2"
// in libpq connection strings or URL query parameters.
var dsnPairRE = regexp.MustCompile(`(?i)\b(password|passwd|pwd)=('[^']*'|[^\s&;]+)`)

// isURLishKey reports whether a key name suggests the value is a URL, DSN,
// or connection string that may embed credentials.
func isURLishKey(k string) bool {
	for _, frag := range [...]string{"url", "uri", "dsn", "conn", "endpoint", "addr"} {
		if asciiContainsFold(k, frag) {
			return true
		}
	}

	return false
}

// scrubValue scrubs URL userinfo from s and, when key suggests a URL or
// DSN, also the scheme-less forms: MySQL "user:pass@tcp(host)/db" userinfo
// and keyword "password=..." pairs.
func scrubValue(key, s string) string {
	out := scrubURLUserinfo(s)
	if !isURLishKey(key) {
		return out
	}

	if !strings.Contains(out, "://") {
		if at := strings.Index(out, "@"); at > 0 {
			if cred := out[:at]; strings.Contains(cred, ":") && !strings.ContainsAny(cred, " /=") {
				out = RedactedValue + out[at:]
			}
		}
	}

	return dsnPairRE.ReplaceAllString(out, "${1}="+RedactedValue)
}

// scrubURLUserinfo replaces the userinfo in s with RedactedValue when s
// is an absolute URL carrying credentials (for example
// "postgres://user:pass@host/db" becomes "postgres://[REDACTED]@host/db").
// The host, path, query, and fragment stay visible for debugging. Values
// without a scheme, host, or userinfo are returned unchanged.
func scrubURLUserinfo(s string) string {
	// Fast path: only parse if it looks like a URL with userinfo.
	// Must contain "://" (scheme) and "@" (userinfo delimiter).
	if !strings.Contains(s, "://") || !strings.Contains(s, "@") {
		return s
	}

	u, err := url.Parse(s)
	if err == nil && u.Scheme != "" && u.Host != "" && u.User != nil {
		var b strings.Builder
		b.WriteString(u.Scheme)
		b.WriteString("://")
		b.WriteString(RedactedValue)
		b.WriteString("@")
		b.WriteString(u.Host)
		if p := u.EscapedPath(); p != "" {
			b.WriteString(p)
		}
		if u.RawQuery != "" {
			b.WriteString("?")
			b.WriteString(u.RawQuery)
		}
		if u.Fragment != "" {
			b.WriteString("#")
			b.WriteString(u.EscapedFragment())
		}

		return b.String()
	}

	// Fallback for inputs url.Parse rejects (for example a MySQL DSN with
	// a func-style host): scrub at the first "@" that follows "://"
	// and precedes any "/", which marks the end of the authority.
	scheme := strings.Index(s, "://")
	if scheme < 0 {
		return s
	}

	rest := s[scheme+3:]
	at := strings.Index(rest, "@")
	if at < 0 {
		return s
	}
	if slash := strings.Index(rest, "/"); slash >= 0 && slash < at {
		return s
	}

	return s[:scheme+3] + RedactedValue + rest[at:]
}
