package config

import (
	"testing"
)

func TestIsSensitiveKey_matrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want bool
	}{
		// Substring rules (reference port).
		{"password", true},
		{"Password", true},
		{"my_password", true},
		{"secret", true},
		{"secret_key", true},
		{"webhook_secret", true},
		{"token", true},
		{"api_token", true},
		{"bearer", true},
		{"bearer_token", true},
		{"credential", true},
		{"my_credential", true},
		{"credentials", true},
		// Exact rules (reference port).
		{"dsn", true},
		{"DSN", true},
		{"api_key", true},
		{"API_KEY", true},
		{"key", true},
		{"KEY", true},
		{"sign_key", true},
		{"encrypt_key", true},
		{"hmac_key", true},
		{"access_key", true},
		{"private_key", true},
		{"client_secret", true},
		// Exact-rule extensions.
		{"auth", true},
		{"AUTH", true},
		{"session", true},
		{"Session", true},
		{"cookie", true},
		{"apikey", true},
		{"APIKEY", true},
		{"passwd", true},
		{"pwd", true},
		// Non-sensitive: must not redact.
		{"addr", false},
		{"url", false},
		{"adapter", false},
		{"region", false},
		{"author", false},
		{"keyboard", false},
		{"monkey", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()

			if got := isSensitiveKey(tc.key); got != tc.want {
				t.Errorf("isSensitiveKey(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestRedactMap_basic(t *testing.T) {
	t.Parallel()

	//nolint:gosec // dummy fixture values are the redaction assertions' subject, not credentials
	m := map[string]any{
		"password":   "hunter2",
		"addr":       "localhost:6379",
		"secret":     "s3cr3t",
		"token":      "tok123",
		"dsn":        "postgres://user:pass@localhost/db",
		"api_key":    "key-123",
		"bearer":     "bearer-xyz",
		"credential": "cred-abc",
		"auth":       "auth-val",
		"session":    "sess-1",
	}
	redactMap(m)

	for _, k := range []string{"password", "secret", "token", "dsn", "api_key", "bearer", "credential", "auth", "session"} {
		if m[k] != RedactedValue {
			t.Errorf("key %q not redacted, got %v", k, m[k])
		}
	}

	if m["addr"] != "localhost:6379" {
		t.Errorf("non-sensitive addr changed: %v", m["addr"])
	}
}

func TestRedactMap_emptyAndNilValuesKept(t *testing.T) {
	t.Parallel()

	m := map[string]any{"password": "", "secret": "", "token": nil}
	redactMap(m)

	if m["password"] != "" {
		t.Errorf("empty password should stay empty, got %v", m["password"])
	}

	if m["secret"] != "" {
		t.Errorf("empty secret should stay empty, got %v", m["secret"])
	}

	if m["token"] != nil {
		t.Errorf("nil token should stay nil, got %v", m["token"])
	}
}

func TestRedactMap_nilMap(t *testing.T) {
	t.Parallel()

	redactMap(nil)
	redactSlice(nil)
}

func TestRedactMap_nestedMap(t *testing.T) {
	t.Parallel()

	m := map[string]any{
		"outer": map[string]any{
			"password": "secret123",
			"nested2": map[string]any{
				"token": "tok",
				"keep":  "value",
			},
		},
		"keep": "ok",
	}
	redactMap(m)

	outer, ok := m["outer"].(map[string]any)
	if !ok {
		t.Fatalf("outer is not a map")
	}

	if outer["password"] != RedactedValue {
		t.Errorf("nested password not redacted: %v", outer["password"])
	}

	n2, ok := outer["nested2"].(map[string]any)
	if !ok {
		t.Fatalf("nested2 is not a map")
	}

	if n2["token"] != RedactedValue {
		t.Errorf("deeply nested token not redacted: %v", n2["token"])
	}

	if n2["keep"] != "value" {
		t.Errorf("non-sensitive nested keep changed: %v", n2["keep"])
	}

	if m["keep"] != "ok" {
		t.Errorf("top-level keep changed: %v", m["keep"])
	}
}

func TestRedactMap_nestedSlice(t *testing.T) {
	t.Parallel()

	m := map[string]any{
		"items": []any{
			map[string]any{"password": "p1", "name": "a"},
			map[string]any{"secret": "s1"},
			[]any{
				map[string]any{"token": "t1"},
			},
		},
		"typed": []map[string]any{
			{"dsn": "postgres://x"},
		},
	}
	redactMap(m)

	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items is not a slice")
	}

	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("items[0] is not a map")
	}

	if first["password"] != RedactedValue {
		t.Errorf("slice map password not redacted: %v", first["password"])
	}

	if first["name"] != "a" {
		t.Errorf("slice map name changed: %v", first["name"])
	}

	second, ok := items[1].(map[string]any)
	if !ok {
		t.Fatalf("items[1] is not a map")
	}

	if second["secret"] != RedactedValue {
		t.Errorf("second slice secret not redacted: %v", second["secret"])
	}

	nestedSlice, ok := items[2].([]any)
	if !ok {
		t.Fatalf("items[2] is not a slice")
	}

	nestedMap, ok := nestedSlice[0].(map[string]any)
	if !ok {
		t.Fatalf("nestedSlice[0] is not a map")
	}

	if nestedMap["token"] != RedactedValue {
		t.Errorf("nested slice token not redacted: %v", nestedMap["token"])
	}

	typed, ok := m["typed"].([]map[string]any)
	if !ok {
		t.Fatalf("typed is not a []map[string]any")
	}

	if typed[0]["dsn"] != RedactedValue {
		t.Errorf("typed slice dsn not redacted: %v", typed[0]["dsn"])
	}
}

func TestRedactMap_nonStringSecret(t *testing.T) {
	t.Parallel()

	m := map[string]any{"password": 12345, "token": []string{"a"}}
	redactMap(m)

	if m["password"] != RedactedValue {
		t.Errorf("non-string password not redacted: %v", m["password"])
	}

	if m["token"] != RedactedValue {
		t.Errorf("non-string token not redacted: %v", m["token"])
	}
}

func TestRedactSlice_direct(t *testing.T) {
	t.Parallel()

	s := []any{
		map[string]any{"password": "p"},
		[]any{map[string]any{"secret": "s"}},
		[]map[string]any{{"dsn": "postgres://x"}},
		"plain",
		42,
		nil,
	}
	redactSlice(s)

	elem, ok := s[0].(map[string]any)
	if !ok || elem["password"] != RedactedValue {
		t.Errorf("slice element password not redacted: %v", s[0])
	}

	inner, ok := s[1].([]any)
	if !ok {
		t.Fatalf("s[1] not []any: %T", s[1])
	}
	innerMap, ok := inner[0].(map[string]any)
	if !ok || innerMap["secret"] != RedactedValue {
		t.Errorf("nested slice secret not redacted: %v", inner[0])
	}

	typed, ok := s[2].([]map[string]any)
	if !ok {
		t.Fatalf("s[2] not []map[string]any: %T", s[2])
	}
	if typed[0]["dsn"] != RedactedValue {
		t.Errorf("typed slice dsn not redacted: %v", typed[0])
	}

	if s[3] != "plain" || s[4] != 42 || s[5] != nil {
		t.Errorf("scalars changed: %v", s[3:])
	}
}

func TestRedact_doesNotMutateCaller(t *testing.T) {
	t.Parallel()

	original := map[string]any{
		"password": "hunter2",
		"addr":     "localhost:6379",
		"nested":   map[string]any{"token": "tok"},
		"items":    []any{map[string]any{"secret": "s"}},
		"typed":    []map[string]any{{"dsn": "postgres://x"}},
	}

	got := Redact(original)

	if got["password"] != RedactedValue {
		t.Errorf("copy password not redacted: %v", got["password"])
	}

	gotTyped, ok := got["typed"].([]map[string]any)
	if !ok {
		t.Fatalf("copy typed not []map[string]any: %T", got["typed"])
	}
	if gotTyped[0]["dsn"] != RedactedValue {
		t.Errorf("copy typed slice dsn not redacted: %v", got["typed"])
	}

	origTyped, ok := original["typed"].([]map[string]any)
	if !ok {
		t.Fatalf("caller typed not []map[string]any: %T", original["typed"])
	}
	if origTyped[0]["dsn"] != "postgres://x" {
		t.Errorf("caller typed slice mutated")
	}

	if original["password"] != "hunter2" {
		t.Errorf("caller map mutated: %v", original["password"])
	}

	origNested, ok := original["nested"].(map[string]any)
	if !ok {
		t.Fatalf("caller nested not map: %T", original["nested"])
	}
	if origNested["token"] != "tok" {
		t.Errorf("caller nested map mutated: %v", original["nested"])
	}

	origItems, ok := original["items"].([]any)
	if !ok {
		t.Fatalf("caller items not []any: %T", original["items"])
	}
	origItem, ok := origItems[0].(map[string]any)
	if !ok {
		t.Fatalf("caller item not map: %T", origItems[0])
	}
	if origItem["secret"] != "s" {
		t.Errorf("caller slice element mutated")
	}

	if got["addr"] != "localhost:6379" {
		t.Errorf("non-sensitive value changed: %v", got["addr"])
	}
}

func TestRedact_deepCopyIndependence(t *testing.T) {
	t.Parallel()

	original := map[string]any{
		"nested": map[string]any{"keep": "v", "list": []any{"a"}},
	}

	got := Redact(original)
	gotNested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf("copy nested not map: %T", got["nested"])
	}
	gotNested["keep"] = "changed"
	gotList, ok := gotNested["list"].([]any)
	if !ok {
		t.Fatalf("copy list not []any: %T", gotNested["list"])
	}
	gotList[0] = "changed"

	origNested, ok := original["nested"].(map[string]any)
	if !ok {
		t.Fatalf("caller nested not map: %T", original["nested"])
	}
	if origNested["keep"] != "v" {
		t.Errorf("mutating copy leaked into original nested map")
	}

	origList, ok := origNested["list"].([]any)
	if !ok {
		t.Fatalf("caller list not []any: %T", origNested["list"])
	}
	if origList[0] != "a" {
		t.Errorf("mutating copy leaked into original nested slice")
	}
}

func TestRedact_scalarsAndTypesPreserved(t *testing.T) {
	t.Parallel()

	original := map[string]any{
		"port":    42,
		"ratio":   1.5,
		"enabled": true,
		"addr":    "x",
	}

	got := Redact(original)

	if got["port"] != 42 || got["ratio"] != 1.5 || got["enabled"] != true || got["addr"] != "x" {
		t.Errorf("scalars changed: %v", got)
	}
}

func TestRedact_nil(t *testing.T) {
	t.Parallel()

	if Redact(nil) != nil {
		t.Errorf("Redact(nil) should return nil")
	}
}

func TestRedactedValue_constant(t *testing.T) {
	t.Parallel()

	if RedactedValue != "[REDACTED]" {
		t.Errorf("RedactedValue = %q, want %q", RedactedValue, "[REDACTED]")
	}
}
