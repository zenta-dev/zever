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

func TestIsSensitiveKey_accessKeyIDGap(t *testing.T) {
	t.Parallel()

	// Documents the denylist gap the zever:"secret" tag closes:
	// "access_key_id" matches no substring rule and no exact rule.
	if isSensitiveKey("access_key_id") {
		t.Error("isSensitiveKey(access_key_id) = true, want false (gap closed by type tag, not key name)")
	}
}

func TestRedactServices_secretTaggedFieldsRedacted(t *testing.T) {
	t.Parallel()

	// Services whose Options carry no time.Duration field: the JSON
	// bridge in serviceToMap cannot marshal durations (pre-existing
	// codec limitation), so Duration-having services redact to an empty
	// map and cannot exercise field-level assertions here.
	cfg := Default()
	cfg.Analytics.Options.APIKey = "sk-analytics"
	cfg.CDN.Options.APIToken = "cdn-token"
	cfg.Crypto.Options.Key = "crypto-key"
	cfg.Crypto.Options.SignKey = "sign-key"
	cfg.Crypto.Options.KeyID = "key-1"
	cfg.Payment.Options.WebhookSecret = "wh-secret"
	cfg.Payment.Options.SecretKey = "stripe-secret"
	cfg.Payment.Options.APIKey = "paddle-key"
	cfg.Billing.Options.SecretKey = "billing-stripe-secret"
	cfg.Billing.Options.APIKey = "billing-paddle-key"
	cfg.Search.Options.APIKey = "sk-search"
	cfg.Secrets.Options.Token = "vault-token"
	cfg.Storage.Options.Secret = "hmac-secret"
	cfg.Storage.Options.AccessKeyID = "AKIAEXAMPLE"
	cfg.Storage.Options.SecretAccessKey = "s3-secret"
	cfg.Storage.Options.Region = "us-west-2"
	cfg.VectorStore.Options.APIKey = "sk-vec"

	services := cfg.RedactedServices()

	flat := []struct{ service, key string }{
		{"analytics", "api_key"},
		{"cdn", "api_token"},
		{"crypto", "key"},
		{"crypto", "sign_key"},
		{"payment", "webhook_secret"},
		{"payment", "secret_key"},
		{"payment", "api_key"},
		{"billing", "secret_key"},
		{"billing", "api_key"},
		{"search", "api_key"},
		{"secrets", "token"},
		{"storage", "secret"},
		{"storage", "access_key_id"},
		{"storage", "secret_access_key"},
		{"vectorstore", "api_key"},
	}
	for _, tc := range flat {
		svc, ok := services[tc.service]
		if !ok {
			t.Errorf("service %q missing from RedactedServices", tc.service)
			continue
		}
		v, ok := svc.Options[tc.key]
		if !ok {
			t.Errorf("%s: key %q missing", tc.service, tc.key)
			continue
		}
		if v != RedactedValue {
			t.Errorf("%s: oddly named secret field %q not redacted, got %v", tc.service, tc.key, v)
		}
	}

	// Controls: non-secret fields stay visible.
	if got := services["crypto"].Options["key_id"]; got != "key-1" {
		t.Errorf("non-secret crypto key_id changed: %v", got)
	}
	if got := services["storage"].Options["region"]; got != "us-west-2" {
		t.Errorf("non-secret storage region changed: %v", got)
	}
}

func TestRedactSecretFields_nestedAndEmbedded(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `json:"secret" zever:"secret"`
		Keep   string `json:"keep"`
	}
	type outer struct {
		Inner inner  `json:"inner"`
		Flat  string `json:"flat" zever:"secret"`
		inner
	}

	m := map[string]any{
		"inner":  map[string]any{"secret": "s", "keep": "k"},
		"flat":   "f",
		"secret": "es",
		"keep":   "ek",
	}
	redactSecretFields(outer{}, m)

	innerMap, ok := m["inner"].(map[string]any)
	if !ok {
		t.Fatalf("inner not a map: %v", m["inner"])
	}
	if innerMap["secret"] != RedactedValue {
		t.Errorf("nested struct secret not redacted: %v", innerMap)
	}
	if innerMap["keep"] != "k" {
		t.Errorf("nested non-secret changed: %v", innerMap)
	}
	if m["flat"] != RedactedValue {
		t.Errorf("flat tagged secret not redacted: %v", m["flat"])
	}
	if m["secret"] != RedactedValue {
		t.Errorf("embedded struct secret not redacted: %v", m["secret"])
	}
	if m["keep"] != "ek" {
		t.Errorf("embedded non-secret changed: %v", m["keep"])
	}
}

func TestRedactSecretFields_secretType(t *testing.T) {
	t.Parallel()

	type typed struct {
		APIKey Secret `json:"api_key" toml:"api_key" yaml:"api_key"`
		Plain  string `json:"plain"`
	}

	m := map[string]any{"api_key": "sk-typed", "plain": "p"}
	redactSecretFields(typed{}, m)

	if m["api_key"] != RedactedValue {
		t.Errorf("config.Secret-typed field not redacted: %v", m["api_key"])
	}
	if m["plain"] != "p" {
		t.Errorf("non-secret field changed: %v", m["plain"])
	}
}

func TestRedactServices_emptySecretStaysEmpty(t *testing.T) {
	t.Parallel()

	cfg := Default()
	services := cfg.RedactedServices()

	if got := services["secrets"].Options["token"]; got != "" {
		t.Errorf("empty secret should stay empty, got %v", got)
	}
}

func TestRedact_scrubsURLUserinfo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"postgres://user:pass@localhost/db", "postgres://[REDACTED]@localhost/db"},
		{"postgres://user:pass@localhost:5432/db?sslmode=disable", "postgres://[REDACTED]@localhost:5432/db?sslmode=disable"},
		{"redis://:pass@localhost:6379/0", "redis://[REDACTED]@localhost:6379/0"},
		{"postgres://user@host/db", "postgres://[REDACTED]@host/db"},
		{"mysql://u:p@tcp(host:3306)/db", "mysql://[REDACTED]@tcp(host:3306)/db"},
		{"postgres://u:p@h/db#frag", "postgres://[REDACTED]@h/db#frag"},
		{"HTTP://user:pass@host/db", "http://[REDACTED]@host/db"},
		{"postgres://host/db", "postgres://host/db"},
		{"localhost:6379", "localhost:6379"},
		{"user:pass@host", "user:pass@host"},
		{"", ""},
	}

	for _, tc := range cases {
		got := Redact(map[string]any{"database_url": tc.in})["database_url"]
		if got != tc.want {
			t.Errorf("Redact userinfo scrub %q = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRedactMap_scrubsURLUserinfo(t *testing.T) {
	t.Parallel()

	//nolint:gosec // dummy fixture values are the redaction assertions' subject, not credentials
	m := map[string]any{"database_url": "postgres://user:pass@localhost/db", "keep": "x"}
	redactMap(m)

	if m["database_url"] != "postgres://[REDACTED]@localhost/db" {
		t.Errorf("redactMap userinfo not scrubbed: %v", m["database_url"])
	}
	if m["keep"] != "x" {
		t.Errorf("non-URL value changed: %v", m["keep"])
	}
}

func TestRedact_denylistStillFullyRedactsDSN(t *testing.T) {
	t.Parallel()

	//nolint:gosec // dummy fixture values are the redaction assertions' subject, not credentials
	got := Redact(map[string]any{"dsn": "postgres://user:pass@localhost/db"})["dsn"]
	if got != RedactedValue {
		t.Errorf("dsn key should fully redact, got %v", got)
	}
}

type secretRoundTripOptions struct {
	Name   string `json:"name" toml:"name" yaml:"name"`
	Secret Secret `json:"secret" toml:"secret" yaml:"secret"`
}

func TestSecret_roundTripDecode(t *testing.T) {
	t.Parallel()

	t.Run("json", func(t *testing.T) {
		t.Parallel()
		path := writeTempConfig(t, "zever.json", `{"ai":{"adapter":"ollama","options":{"name":"n","secret":"s3cr3t"}}}`)
		got, err := decodeFile(path)
		if err != nil {
			t.Fatalf("decodeFile: %v", err)
		}
		opts, err := decodeOptions[secretRoundTripOptions]("ai", got["ai"].Options)
		if err != nil {
			t.Fatalf("json strict decode: %v", err)
		}
		if opts.Secret != "s3cr3t" {
			t.Errorf("json decode of Secret = %q, want s3cr3t", opts.Secret)
		}
	})

	t.Run("yaml", func(t *testing.T) {
		t.Parallel()
		path := writeTempConfig(t, "zever.yaml", "ai:\n  adapter: ollama\n  options:\n    name: n\n    secret: s3cr3t\n")
		got, err := decodeFile(path)
		if err != nil {
			t.Fatalf("decodeFile: %v", err)
		}
		opts, err := decodeOptions[secretRoundTripOptions]("ai", got["ai"].Options)
		if err != nil {
			t.Fatalf("yaml strict decode: %v", err)
		}
		if opts.Secret != "s3cr3t" {
			t.Errorf("yaml decode of Secret = %q, want s3cr3t", opts.Secret)
		}
	})

	t.Run("env", func(t *testing.T) {
		t.Parallel()
		var opts secretRoundTripOptions
		if err := setOptionField("x", &opts, []string{"secret"}, "s3cr3t", "X_SECRET"); err != nil {
			t.Fatalf("setOptionField: %v", err)
		}
		if opts.Secret != "s3cr3t" {
			t.Errorf("env decode of Secret = %q, want s3cr3t", opts.Secret)
		}
	})
}

func TestSecret_strictDecodeStaysStrict(t *testing.T) {
	t.Parallel()

	_, err := decodeOptions[secretRoundTripOptions]("x", map[string]any{
		"name":          "n",
		"secret":        "s3cr3t",
		"unknown_field": "v",
	})
	if err == nil {
		t.Error("unknown field should fail strict decode")
	}

	_, err = decodeOptions[secretRoundTripOptions]("x", map[string]any{"secret": 42})
	if err == nil {
		t.Error("non-string secret should fail strict decode")
	}

	got, err := decodeOptions[secretRoundTripOptions]("x", map[string]any{"name": "n", "secret": "s3cr3t"})
	if err != nil {
		t.Fatalf("strict decode of valid options: %v", err)
	}
	if got.Secret != "s3cr3t" {
		t.Errorf("Secret = %q, want s3cr3t", got.Secret)
	}
}
