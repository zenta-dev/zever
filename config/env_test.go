package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEnvAdapterAll(t *testing.T) {
	cfg := Default()
	for _, svc := range knownServiceNames() {
		t.Setenv(svcKey(svc, "ADAPTER"), "custom-"+svc)
	}
	if err := applyEnv(cfg); err != nil {
		t.Fatal(err)
	}
	for _, svc := range knownServiceNames() {
		adapter, _, _ := serviceRefs(cfg, svc)
		if *adapter != "custom-"+svc {
			t.Fatalf("%s: got %q", svc, *adapter)
		}
	}
}

func svcKey(svc, field string) string {
	return strings.ToUpper(svc) + "_" + field
}

func TestEnvFieldTypes(t *testing.T) {
	t.Setenv("DB_MAXCONNS", "7")
	t.Setenv("BILLING_SANDBOX", "true")
	t.Setenv("RATELIMIT_RATE", "2.5")
	t.Setenv("DB_MAXCONNLIFETIME", "5m")
	t.Setenv("ROUTER_APPNAME", "myapp")
	t.Setenv("PASSWORD_TIME", "5")
	t.Setenv("AUTH_JWT_SECRET", "dummy-secret-value")
	cfg := Default()
	if err := applyEnv(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 7 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
	if !cfg.Billing.Options.Sandbox {
		t.Fatal("want sandbox true")
	}
	if cfg.Ratelimit.Options.Rate != 2.5 {
		t.Fatalf("got %v", cfg.Ratelimit.Options.Rate)
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
	t.Setenv("SESSION_REDIS_ADDR", "localhost:6379")
	t.Setenv("FLAG_STATIC_PATH", "/tmp/x")
	t.Setenv("WEBHOOK_MAX_RETRIES", "3")
	t.Setenv("PASSWORD_SALT_LENGTH", "24")
	t.Setenv("CACHE_MAXENTRIES", "100")
	t.Setenv("STORAGE_ROOT", "/data")
	t.Setenv("CRYPTO_SIGN_KEY", "e30=")
	t.Setenv("SESSION_TTL", "30s")
	cfg := Default()
	if err := applyEnv(cfg); err != nil {
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
	if cfg.Crypto.Options.SignKey != "e30=" {
		t.Fatalf("got %q", cfg.Crypto.Options.SignKey)
	}
	if cfg.Session.Options.TTL != 30*time.Second {
		t.Fatalf("got %v", cfg.Session.Options.TTL)
	}
}

func TestEnvIgnoreUnknown(t *testing.T) {
	t.Setenv("FOO_BAR", "x")
	t.Setenv("AUTH_TOKEN", "hostile")
	t.Setenv("DB", "hostile")
	t.Setenv("CONFIGTESTBARE", "hostile")
	t.Setenv("DB_", "x")
	t.Setenv("DB_NOPE_X", "x")
	t.Setenv("PERMISSION_RULES", "x")
	t.Setenv("OBSERVABILITY_HEADERS", "x")
	t.Setenv("SCHEDULER_DISPATCHER_Q", "x")
	t.Setenv("STORAGE_POLICY_X", "y")
	t.Setenv("DB_DSN_X", "z")
	t.Setenv("EVENTBUS_ONPANIC", "x")
	t.Setenv("SCHEDULER_DISPATCHER", "x")
	cfg := Default()
	before := *cfg
	if err := applyEnv(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.DSN != "" || cfg.DB.Adapter != "sqlite" {
		t.Fatal("hostile env leaked into db")
	}
	if cfg.Auth.Options.JWT.Secret != "" {
		t.Fatal("AUTH_TOKEN leaked into auth")
	}
	_ = before
}

func TestEnvTypeErrors(t *testing.T) {
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
			t.Setenv(key, val)
			err := applyEnv(Default())
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
