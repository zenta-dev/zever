package config

import (
	"errors"
	"strings"
	"testing"
)

// TestIsSensitiveKey_nonASCIIFallback covers the non-ASCII path through
// isSensitiveKey, which lowers the key before applying the substring and
// exact rules (isSensitiveKeyLower). ASCII keys never reach it.
func TestIsSensitiveKey_nonASCIIFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want bool
	}{
		{"password_ü", true},     // substring rule after lowering
		{"ü_secret", true},       // substring rule after lowering
		{"\u212aey", true},       // Kelvin sign K folds to ASCII "key" (exact rule)
		{"üser", false},          // non-ASCII, no rule matches
		{"\u212aeyboard", false}, // folds to "keyboard", not exact and no substring
		{"PÄSSWORD", false},      // lower form "pässword" contains no ASCII rule
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()

			if got := isSensitiveKey(tc.key); got != tc.want {
				t.Fatalf("isSensitiveKey(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

// TestRedactStringSlice_direct covers the header-list redaction helper:
// "key: value" entries whose key is sensitive are rewritten in place;
// non-sensitive keys, entries without the ": " separator, and entries with
// an empty value are left untouched.
func TestRedactStringSlice_direct(t *testing.T) {
	t.Parallel()

	s := []string{
		"X-Auth-Token: abc",  // key contains "token" -> redacted
		"Cookie: sid=1",      // exact "cookie" rule -> redacted
		"Content-Type: json", // non-sensitive key -> untouched
		"NoSeparator",        // no ": " -> untouched
		"Token:",             // empty value -> untouched
	}
	redactStringSlice(s)

	want := []string{
		"X-Auth-Token: " + RedactedValue,
		"Cookie: " + RedactedValue,
		"Content-Type: json",
		"NoSeparator",
		"Token:",
	}
	for i := range want {
		if s[i] != want[i] {
			t.Errorf("s[%d] = %q, want %q", i, s[i], want[i])
		}
	}
}

// TestRedactCopyValue_branches exercises every type branch of the deep-copy
// redactor directly, including the sensitive empty-string/nil carve-outs and
// the nested slice/map recursion.
func TestRedactCopyValue_branches(t *testing.T) {
	t.Parallel()

	if got := redactCopyValue("", true); got != "" {
		t.Fatalf("sensitive empty string = %v, want empty", got)
	}
	if got := redactCopyValue(nil, true); got != nil {
		t.Fatalf("sensitive nil = %v, want nil", got)
	}
	if got := redactCopyValue(42, true); got != RedactedValue {
		t.Fatalf("sensitive non-string = %v, want %q", got, RedactedValue)
	}

	nested, ok := redactCopyValue(map[string]any{"token": "t", "keep": "v"}, false).(map[string]any)
	if !ok || nested["token"] != RedactedValue || nested["keep"] != "v" {
		t.Fatalf("nested map copy = %#v", nested)
	}

	maps, ok := redactCopyValue([]map[string]any{{"secret": "s"}, {"keep": "v"}}, false).([]map[string]any)
	if !ok || maps[0]["secret"] != RedactedValue || maps[1]["keep"] != "v" {
		t.Fatalf("[]map copy = %#v", maps)
	}

	list, ok := redactCopyValue([]any{map[string]any{"password": "p"}, "plain"}, false).([]any)
	if !ok {
		t.Fatalf("[]any copy type = %T", list)
	}
	lm, ok := list[0].(map[string]any)
	if !ok || lm["password"] != RedactedValue || list[1] != "plain" {
		t.Fatalf("[]any copy = %#v", list)
	}

	headers, ok := redactCopyValue([]string{"X-Auth-Token: abc", "Content-Type: json"}, false).([]string)
	if !ok || headers[0] != "X-Auth-Token: "+RedactedValue || headers[1] != "Content-Type: json" {
		t.Fatalf("[]string copy = %#v", headers)
	}

	if got := redactCopyValue(7, false); got != 7 {
		t.Fatalf("scalar copy = %v, want 7", got)
	}
	if got := redactCopyValue(nil, false); got != nil {
		t.Fatalf("nil copy = %v, want nil", got)
	}
}

// TestDecodePluginsEntry_shapes covers the two accepted plugin block shapes
// plus the malformed, type-mismatch, and unmarshalable error paths.
func TestDecodePluginsEntry_shapes(t *testing.T) {
	t.Parallel()

	// Direct map: plugin names map to their own envelopes.
	direct, err := decodePluginsEntry("plugins", map[string]any{
		"myplug": map[string]any{"adapter": "custom"},
	})
	if err != nil {
		t.Fatalf("direct map: %v", err)
	}
	if direct.Adapter != "" {
		t.Fatalf("direct map adapter = %q, want empty", direct.Adapter)
	}
	if _, ok := direct.Options["myplug"]; !ok {
		t.Fatalf("direct map options = %#v, want myplug key", direct.Options)
	}

	// Uniform envelope.
	envelope, err := decodePluginsEntry("plugins", map[string]any{
		"adapter": "custom",
		"options": map[string]any{"myplug": map[string]any{"adapter": "custom"}},
	})
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if envelope.Adapter != "custom" {
		t.Fatalf("envelope adapter = %q, want custom", envelope.Adapter)
	}

	// Empty block decodes to the zero envelope.
	empty, err := decodePluginsEntry("plugins", map[string]any{})
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if empty.Adapter != "" || len(empty.Options) != 0 {
		t.Fatalf("empty = %#v, want zero", empty)
	}

	// Non-envelope with a malformed plugin entry is rejected.
	if _, err := decodePluginsEntry("plugins", map[string]any{"bad": 5}); err == nil {
		t.Fatal("malformed plugin entry = nil error, want error")
	}

	// Envelope with a type mismatch is a scrubbed DecodeError.
	var de *DecodeError
	if _, err := decodePluginsEntry("plugins", map[string]any{"adapter": 123}); !errors.As(err, &de) {
		t.Fatalf("type mismatch err = %v, want DecodeError", err)
	}

	// Unmarshalable entry surfaces a DecodeError rather than panicking.
	if _, err := decodePluginsEntry("plugins", map[string]any{"x": make(chan int)}); !errors.As(err, &de) {
		t.Fatalf("unmarshalable entry err = %v, want DecodeError", err)
	}
}

// TestIsEnvelopeKeys_boundaries covers the envelope-key classifier, including
// the nil/empty maps that count as envelopes.
func TestIsEnvelopeKeys_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		m    map[string]any
		want bool
	}{
		{"nil", nil, true},
		{"empty", map[string]any{}, true},
		{"adapter only", map[string]any{"adapter": "x"}, true},
		{"options only", map[string]any{"options": nil}, true},
		{"both", map[string]any{"adapter": "x", "options": nil}, true},
		{"other", map[string]any{"other": 1}, false},
		{"adapter plus other", map[string]any{"adapter": "x", "other": 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isEnvelopeKeys(tc.m); got != tc.want {
				t.Fatalf("isEnvelopeKeys(%v) = %v, want %v", tc.m, got, tc.want)
			}
		})
	}
}

// TestNormNameAndNormEqualFold covers the allocation-free normalization
// comparison: ASCII fold, "_" elision, underscore boundaries, and the
// non-ASCII fallback.
func TestNormNameAndNormEqualFold(t *testing.T) {
	t.Parallel()

	if got := normName("Max_Retries"); got != "maxretries" {
		t.Fatalf("normName = %q, want maxretries", got)
	}
	if !normEqualFold("Max_Retries", "maxretries") {
		t.Fatal("normEqualFold should fold case and underscores")
	}
	if normEqualFold("max", "min") {
		t.Fatal("normEqualFold(max, min) = true, want false")
	}
	if !normEqualFold("_max_", "max") {
		t.Fatal("normEqualFold should ignore leading/trailing underscores")
	}
	if normEqualFold("max", "maxx") {
		t.Fatal("normEqualFold(max, maxx) = true, want false")
	}
	if !normEqualFold("mäx", "MÄX") {
		t.Fatal("normEqualFold non-ASCII fallback should fold case")
	}
	if normEqualFold("mäx", "max") {
		t.Fatal("normEqualFold non-ASCII vs ASCII = true, want false")
	}
}

// TestScrubbedValue_namesFieldNotValue pins the secret-safe error contract:
// only the field name, optional env var, and expected kind survive.
func TestScrubbedValue_namesFieldNotValue(t *testing.T) {
	t.Parallel()

	err := scrubbedValue("MaxConns", "DB_MAXCONNS", "int")
	if !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("err = %v, want ErrInvalidValue", err)
	}
	if !strings.Contains(err.Error(), "DB_MAXCONNS") || !strings.Contains(err.Error(), `"MaxConns"`) {
		t.Fatalf("err = %v, want field and env name", err)
	}

	noEnv := scrubbedValue("MaxConns", "", "int")
	if !errors.Is(noEnv, ErrInvalidValue) {
		t.Fatalf("err = %v, want ErrInvalidValue", noEnv)
	}
	if strings.Contains(noEnv.Error(), " from env ") {
		t.Fatalf("err = %v, want no env clause when env name empty", noEnv)
	}
}
