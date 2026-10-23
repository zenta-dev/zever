package config

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestEnvAdapterKeysRejected pins the removal across every service: a
// SERVICE_ADAPTER key for any known service fails resolution with an
// UnknownFieldError carrying the removal message.
func TestEnvAdapterKeysRejected(t *testing.T) {
	t.Parallel()
	for _, svc := range knownServiceNames() {
		t.Run(svc, func(t *testing.T) {
			t.Parallel()
			key := strings.ToUpper(svc) + "_ADAPTER"
			cfg, err := Resolve(Env{key: "custom"})
			if err == nil {
				t.Fatalf("%s: want error", key)
			}
			if cfg != nil {
				t.Fatalf("%s: want nil config", key)
			}
			var unknown *UnknownFieldError
			if !errors.As(err, &unknown) {
				t.Fatalf("%s: want UnknownFieldError, got %T: %v", key, err, err)
			}
			if unknown.Suggestion != "" {
				t.Fatalf("%s: suggestion = %q, want empty", key, unknown.Suggestion)
			}
			if !strings.Contains(err.Error(), "adapter selection removed") {
				t.Fatalf("%s: error = %v, want removal message", key, err)
			}
		})
	}
}

// TestEnvAdapterLikeOptionStillApplies pins the carve-out: a key ending in
// _ADAPTER that resolves to a real settable option field applies normally
// instead of failing as a removed adapter key.
func TestEnvAdapterLikeOptionStillApplies(t *testing.T) {
	t.Parallel()
	cfg, err := Resolve(Env{"WEBHOOK_QUEUE_ADAPTER": "custom-queue"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Webhook.Options.QueueAdapter != "custom-queue" {
		t.Fatalf("got %q", cfg.Webhook.Options.QueueAdapter)
	}
}

func TestEnvFieldTypes(t *testing.T) {
	t.Parallel()
	env := Env{
		"DB_MAXCONNS":        "7",
		"BILLING_SANDBOX":    "true",
		"RATELIMIT_RATE":     "2.5",
		"DB_MAXCONNLIFETIME": "5m",
		"ROUTER_APPNAME":     "myapp",
		"PASSWORD_TIME":      "5",
		"AUTH_JWT_SECRET":    "dummy-secret-value",
	}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 7 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
	if !cfg.Billing.Options.Sandbox {
		t.Fatal("want sandbox true")
	}
	if cfg.RateLimit.Options.Rate != 2.5 {
		t.Fatalf("got %v", cfg.RateLimit.Options.Rate)
	}
	if cfg.DB.Options.MaxConnLifetime != 5*time.Minute {
		t.Fatalf("got %v", cfg.DB.Options.MaxConnLifetime)
	}
	if cfg.Router.Options.AppName != "myapp" {
		t.Fatalf("got %q", cfg.Router.Options.AppName)
	}
	if cfg.Password.Options.Time != 5 {
		t.Fatalf("got %d", cfg.Password.Options.Time)
	}
	if cfg.Auth.Options.JWT.Secret != "dummy-secret-value" {
		t.Fatal("nested jwt secret not set")
	}
}

func TestEnvNested(t *testing.T) {
	t.Parallel()
	env := Env{
		"SESSION_REDIS_ADDR":   "localhost:6379",
		"FLAG_STATIC_PATH":     "/tmp/x",
		"WEBHOOK_MAX_RETRIES":  "3",
		"PASSWORD_SALT_LENGTH": "24",
		"CACHE_MAXENTRIES":     "100",
		"STORAGE_ROOT":         "/data",
		"SESSION_TTL":          "30s",
	}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Session.Options.Redis.Addr != "localhost:6379" {
		t.Fatalf("got %q", cfg.Session.Options.Redis.Addr)
	}
	if cfg.Flag.Options.Static.Path != "/tmp/x" {
		t.Fatalf("got %q", cfg.Flag.Options.Static.Path)
	}
	if cfg.Webhook.Options.MaxRetries != 3 {
		t.Fatalf("got %d", cfg.Webhook.Options.MaxRetries)
	}
	if cfg.Password.Options.SaltLen != 24 {
		t.Fatalf("got %d", cfg.Password.Options.SaltLen)
	}
	if cfg.Cache.Options.MaxEntries != 100 {
		t.Fatalf("got %d", cfg.Cache.Options.MaxEntries)
	}
	if cfg.Storage.Options.Root != "/data" {
		t.Fatalf("got %q", cfg.Storage.Options.Root)
	}
	if cfg.Session.Options.TTL != 30*time.Second {
		t.Fatalf("got %v", cfg.Session.Options.TTL)
	}
}

func TestEnvCryptoSignKey(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{"CRYPTO_SIGN_KEY": "e30="}
	if err := ApplyEnv(env, "crypto", "", &cfg.Crypto.Options); err != nil {
		t.Fatal(err)
	}
	if cfg.Crypto.Options.SignKey != "e30=" {
		t.Fatalf("got %q", cfg.Crypto.Options.SignKey)
	}
}

func TestEnvDedicatedPool(t *testing.T) {
	t.Parallel()
	env := Env{
		"SEARCH_DEDICATEDPOOL": "true",
		"QUEUE_DEDICATEDPOOL":  "true",
		"SESSION_DSN":          "data/sess.db",
	}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Search.Options.DedicatedPool {
		t.Error("SEARCH_DEDICATEDPOOL not applied")
	}
	if !cfg.Queue.Options.DedicatedPool {
		t.Error("QUEUE_DEDICATEDPOOL not applied")
	}
	if cfg.Session.Options.DSN != "data/sess.db" {
		t.Errorf("SESSION_DSN = %q, want data/sess.db", cfg.Session.Options.DSN)
	}
}

// TestEnvIgnoreUnknownHeads pins the ambient-environment rule: keys whose
// head names no known service (or which name no field at all) never break
// resolution.
func TestEnvIgnoreUnknownHeads(t *testing.T) {
	t.Parallel()
	env := Env{
		"FOO_BAR":        "x",
		"PATH":           "/bin",
		"DB":             "hostile",
		"CONFIGTESTBARE": "hostile",
		"DB_":            "x",
	}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.DSN != "" {
		t.Fatal("hostile env leaked into db")
	}
}

// TestEnvUnknownFieldFails pins the fail-fast rule for typos: a key under a
// WIRED service prefix that is close to a known field is an error, not a
// silent skip, so typos surface instead of booting with defaults. The
// error carries a did-you-mean hint.
//
// Keys that are close to NO known field are ambient variables and are
// ignored instead (see TestEnvIgnoresAmbientVariables); StrictEnvKey turns
// them back into errors (see TestEnvStrictModeRejectsAmbientKeys).
func TestEnvUnknownFieldFails(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"DB_MAXCONN", "AUTH_JWT_SECRT", "PERMISSION_RULES",
		"OBSERVABILITY_HEADERS", "STORAGE_POLICY_X", "DB_DSN_X",
	} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve(Env{key: "x"})
			if err == nil {
				t.Fatalf("%s: want error", key)
			}
			var unknown *UnknownFieldError
			if !errors.As(err, &unknown) {
				t.Fatalf("%s: want UnknownFieldError, got %T: %v", key, err, err)
			}
			if !errors.Is(err, ErrUnknownField) {
				t.Fatalf("%s: want ErrUnknownField match: %v", key, err)
			}
			if unknown.Suggestion == "" {
				t.Fatalf("%s: want a did-you-mean suggestion: %v", key, err)
			}
			want := fmt.Sprintf("config: unknown field %q for service %q (did you mean %q?)",
				unknown.Field, unknown.Service, unknown.Suggestion)
			if err.Error() != want {
				t.Fatalf("%s: error = %q, want %q", key, err.Error(), want)
			}
		})
	}
}

func TestEnvTypeErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"DB_MAXCONNS":               "abc",
		"BILLING_SANDBOX":           "maybe",
		"RATELIMIT_RATE":            "abc",
		"DB_MAXCONNLIFETIME":        "bogus",
		"PASSWORD_THREADS":          "999",
		"AUTH_JWT_MAXTTL":           "bogus",
		"WEBHOOK_MAX_RETRIES":       "abc",
		"DB_MAXCONNIDLETIME":        "bogus",
		"PASSWORD_MEMORY":           "abc",
		"OBSERVABILITY_SAMPLERATIO": "abc",
	}
	for key, val := range cases {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			_, err := Resolve(Env{key: val})
			if err == nil {
				t.Fatal("want error")
			}
			var invalid *InvalidOptionsError
			if !errors.As(err, &invalid) {
				t.Fatalf("want InvalidOptionsError, got %T: %v", err, err)
			}
			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("want ErrInvalidOptions match: %v", err)
			}
		})
	}
}

func TestServiceRefsUnknown(t *testing.T) {
	t.Parallel()
	_, ok := serviceRefs(Default(), "nope")
	if ok {
		t.Fatal("want not ok")
	}
}

func TestSetOptionFieldDirect(t *testing.T) {
	t.Parallel()
	type inner struct {
		Name string
	}
	type synthetic struct {
		hidden string
		Shown  string
		Tagged string `json:"custom_name"`
		Empty  string `json:",omitempty"`
		Nested inner
		Nil    *inner
	}
	s := &synthetic{Nested: inner{}}
	if err := setOptionField("svc", s, []string{"shown"}, "v"); err != nil {
		t.Fatal(err)
	}
	if s.Shown != "v" {
		t.Fatalf("got %q", s.Shown)
	}
	if err := setOptionField("svc", s, []string{"CUSTOM_NAME"}, "w"); err != nil {
		t.Fatal(err)
	}
	if s.Tagged != "w" {
		t.Fatalf("got %q", s.Tagged)
	}
	if err := setOptionField("svc", s, []string{"empty"}, "e"); err != nil {
		t.Fatal(err)
	}
	if s.Empty != "e" {
		t.Fatalf("got %q", s.Empty)
	}
	var unknown *UnknownFieldError
	if err := setOptionField("svc", s, []string{"hidden"}, "v"); !errors.As(err, &unknown) {
		t.Fatalf("want UnknownFieldError, got %v", err)
	}
	if s.hidden != "" {
		t.Fatalf("unexported field was set: %q", s.hidden)
	}
	if err := setOptionField("svc", s, []string{"nested", "name"}, "n"); err != nil {
		t.Fatal(err)
	}
	if s.Nested.Name != "n" {
		t.Fatalf("got %q", s.Nested.Name)
	}
	if err := setOptionField("svc", s, []string{"nil", "name"}, "n"); !errors.As(err, &unknown) {
		t.Fatalf("want UnknownFieldError, got %v", err)
	}
	if err := setOptionField("svc", *s, []string{"shown"}, "v"); !errors.As(err, &unknown) {
		t.Fatalf("want UnknownFieldError, got %v", err)
	}
	n := 1
	if err := setOptionField("svc", &n, []string{"shown"}, "v"); !errors.As(err, &unknown) {
		t.Fatalf("want UnknownFieldError, got %v", err)
	}
}

// TestCamelToSnake pins the CamelCase -> snake_case conversion used by
// jsonFieldName for untagged Options fields. Expectations are derived from the
// implementation in env.go, not from a spec: an uppercase ASCII rune inserts
// "_" when the previous byte is lowercase ASCII or the next byte is lowercase
// ASCII, and is otherwise lowercased with no separator. Digits are written
// verbatim and never trigger a separator, so an uppercase letter is only split
// off a digit run when the following letter is lowercase (DB2 -> db2,
// DB2Conn -> db2_conn). Non-ASCII runes are written unchanged: they are never
// lowercased and never index prev/next, so non-ASCII input cannot panic.
func TestCamelToSnake(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"A", "a"},
		{"API", "api"},
		{"APIKey", "api_key"},
		{"dbMaxConns", "db_max_conns"},
		{"HTTPServer", "http_server"},
		{"OIDCIssuerURL", "oidc_issuer_url"},
		{"ABCDef", "abc_def"},
		{"DB2", "db2"},
		{"DB2Conn", "db2_conn"},
		{"Max2Retries", "max2_retries"},
		{"Ünicode", "Ünicode"},
		{"caféAPIKey", "caféapi_key"},
	}
	for _, tt := range tests {
		if got := camelToSnake(tt.in); got != tt.want {
			t.Errorf("camelToSnake(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
