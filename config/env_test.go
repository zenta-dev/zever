package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEnvAdapterAll(t *testing.T) {
	t.Parallel()
	env := make(Env, len(knownServiceNames()))
	for _, svc := range knownServiceNames() {
		env[strings.ToUpper(svc)+"_ADAPTER"] = "custom-" + svc
	}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range knownServiceNames() {
		adapter, _, _ := serviceRefs(cfg, svc)
		if *adapter != "custom-"+svc {
			t.Fatalf("%s: got %q", svc, *adapter)
		}
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
	if err := ApplyEnv(env, "crypto", "", &cfg.Crypto); err != nil {
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
	if cfg.DB.Options.DSN != "" || cfg.DB.Adapter != "sqlite" {
		t.Fatal("hostile env leaked into db")
	}
}

// TestEnvUnknownFieldFails pins the fail-fast rule: a key under a WIRED
// service prefix that resolves to no field is an error, not a silent skip,
// so typos surface instead of booting with defaults.
func TestEnvUnknownFieldFails(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"DB_NOPE_X", "AUTH_TOKEN", "PERMISSION_RULES",
		"OBSERVABILITY_HEADERS", "SCHEDULER_DISPATCHER_Q",
		"STORAGE_POLICY_X", "DB_DSN_X", "EVENTBUS_ONPANIC", "SCHEDULER_DISPATCHER",
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
	_, _, ok := serviceRefs(Default(), "nope")
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
