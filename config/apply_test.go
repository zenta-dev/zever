package config

import (
	"errors"
	"strings"
	"testing"
)

func TestApplyEnvDefaultInstance(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{"DB_MAXCONNS": "7", "DB_ADAPTER": "postgres", "AUTH_JWT_SECRET": "dummy-secret-value"}
	if err := ApplyEnv(env, "db", "", &cfg.DB); err != nil {
		t.Fatal(err)
	}
	if err := ApplyEnv(env, "auth", "", &cfg.Auth); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 7 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
	if cfg.Auth.Options.JWT.Secret != "dummy-secret-value" {
		t.Fatal("nested jwt secret not set")
	}
}

func TestApplyEnvBareOptions(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{"DB_MAXCONNS": "7", "DB_ADAPTER": "postgres"}
	if err := ApplyEnv(env, "db", "", &cfg.DB.Options); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 7 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
	if cfg.DB.Adapter != "sqlite" {
		t.Fatalf("adapter must not leak into bare options: %q", cfg.DB.Adapter)
	}
}

func TestApplyEnvCaseInsensitive(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{"db_maxconns": "9", "Db_Adapter": "postgres"}
	if err := ApplyEnv(env, "DB", "", &cfg.DB); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 9 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
}

func TestApplyEnvIgnoresOtherServices(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{
		"FOO_BAR": "x", "PATH": "/bin", "DB": "hostile",
		"CONFIGTESTBARE": "hostile", "DB_": "x",
		"CACHE_MAXENTRIES": "100",
	}
	if err := ApplyEnv(env, "db", "", &cfg.DB); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 0 || cfg.DB.Adapter != "sqlite" {
		t.Fatal("hostile env leaked into db")
	}
	if cfg.Cache.Options.MaxEntries != 0 {
		t.Fatal("other service key leaked")
	}
}

func TestApplyEnvIgnoresInstanceKeys(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{"DB__REPORTING__DSN": "postgres://x", "DB__REPORTING__ADAPTER": "postgres"}
	if err := ApplyEnv(env, "db", "", &cfg.DB); err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.DSN != "" {
		t.Fatalf("instance key leaked into default: %q", cfg.DB.Options.DSN)
	}
}

func TestApplyEnvUnknownKeyErrorsWithSuggestion(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{"DB_MAXCONBNS": "7"}
	err := ApplyEnv(env, "db", "", &cfg.DB)
	if err == nil {
		t.Fatal("want error")
	}
	var unknown *UnknownFieldError
	if !errors.As(err, &unknown) {
		t.Fatalf("want UnknownFieldError, got %T: %v", err, err)
	}
	if !errors.Is(err, ErrUnknownField) {
		t.Fatalf("want ErrUnknownField match: %v", err)
	}
	if unknown.Suggestion != "max_conns" {
		t.Fatalf("suggestion = %q, want max_conns", unknown.Suggestion)
	}
}

func TestApplyEnvTypeErrorScrubbed(t *testing.T) {
	t.Parallel()
	secret := "super-secret-value-xyz-123"
	cfg := Default()
	env := Env{"DB_MAXCONNS": secret}
	err := ApplyEnv(env, "db", "", &cfg.DB)
	if err == nil {
		t.Fatal("want error")
	}
	var invalid *InvalidOptionsError
	if !errors.As(err, &invalid) {
		t.Fatalf("want InvalidOptionsError, got %T: %v", err, err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error echoes value: %v", err)
	}
}

func TestApplyEnvUnknownService(t *testing.T) {
	t.Parallel()
	cfg := Default()
	err := ApplyEnv(Env{"NOPE_X": "1"}, "nope", "", &cfg.DB)
	if err == nil {
		t.Fatal("want error")
	}
	if !errors.Is(err, ErrUnknownService) {
		t.Fatalf("want ErrUnknownService match: %v", err)
	}
}

func TestApplyEnvBadDst(t *testing.T) {
	t.Parallel()
	for _, dst := range []any{nil, 42, "x"} {
		if err := ApplyEnv(Env{"DB_MAXCONNS": "1"}, "db", "", dst); err == nil {
			t.Fatalf("want error for dst %T", dst)
		}
	}
	var n int
	if err := ApplyEnv(Env{"DB_MAXCONNS": "1"}, "db", "", &n); err == nil {
		t.Fatal("want error for non-struct dst")
	}
}

func TestApplyEnvNamedInstance(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{
		"DB__REPORTING__DSN":     "postgres://reporting/db",
		"DB__REPORTING__ADAPTER": "postgres",
		"DB_DSN":                 "sqlite://default",
	}
	reporting := cfg.DB
	if err := ApplyEnv(env, "db", "reporting", &reporting); err != nil {
		t.Fatal(err)
	}
	if reporting.Options.DSN != "postgres://reporting/db" {
		t.Fatalf("got %q", reporting.Options.DSN)
	}
	if reporting.Adapter != "postgres" {
		t.Fatalf("got %q", reporting.Adapter)
	}
	if cfg.DB.Options.DSN != "" {
		t.Fatal("named key leaked into default")
	}
}

func TestApplyEnvNamedInstanceCaseInsensitive(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{"db__reporting__maxconns": "11"}
	svc := cfg.DB
	if err := ApplyEnv(env, "db", "REPORTING", &svc); err != nil {
		t.Fatal(err)
	}
	if svc.Options.MaxConns != 11 {
		t.Fatalf("got %d", svc.Options.MaxConns)
	}
}

func TestApplyEnvNamedUnknownKeyErrors(t *testing.T) {
	t.Parallel()
	cfg := Default()
	env := Env{"DB__REPORTING__BOGUS": "1"}
	err := ApplyEnv(env, "db", "reporting", &cfg.DB)
	if err == nil {
		t.Fatal("want error")
	}
	var unknown *UnknownFieldError
	if !errors.As(err, &unknown) {
		t.Fatalf("want UnknownFieldError, got %T: %v", err, err)
	}
}

func TestApplyEnvDoubleUnderscoreNameRejected(t *testing.T) {
	t.Parallel()
	cfg := Default()
	if err := ApplyEnv(Env{}, "db", "a__b", &cfg.DB); err == nil {
		t.Fatal("want error for name containing __")
	}
}

func TestApplyEnvCollisionErrors(t *testing.T) {
	t.Parallel()
	type nested struct {
		Secret string `json:"secret"`
	}
	type colliding struct {
		JWT       nested `json:"jwt"`
		JWTSecret string `json:"jwt_secret"`
	}
	dst := &colliding{}
	err := ApplyEnv(Env{"DB_JWT_SECRET": "canary-collision-value"}, "db", "", dst)
	if err == nil {
		t.Fatal("want collision error: key matches flat jwt_secret and nested jwt.secret")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("want ErrInvalidOptions match: %v", err)
	}
	if strings.Contains(err.Error(), "canary-collision-value") {
		t.Fatalf("error echoes value: %v", err)
	}
}

func TestInstanceNames(t *testing.T) {
	t.Parallel()
	env := Env{
		"DB__REPORTING__DSN":     "x",
		"DB__REPORTING__ADAPTER": "postgres",
		"DB__ANALYTICS__DSN":     "y",
		"db__lower__dsn":         "z",
		"DB_DSN":                 "default",
		"DB__":                   "empty",
		"DB__NONAME":             "nofield",
		"CACHE__C1__URL":         "u",
	}
	got := InstanceNames(env, "db")
	want := []string{"ANALYTICS", "LOWER", "REPORTING"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if out := InstanceNames(env, "nope"); len(out) != 0 {
		t.Fatalf("unknown service: got %v", out)
	}
	if out := InstanceNames(nil, "db"); len(out) != 0 {
		t.Fatalf("nil env: got %v", out)
	}
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()
	for _, svc := range knownServiceNames() {
		opts := DefaultOptions(svc, "")
		if opts == nil {
			t.Fatalf("%s: got nil", svc)
		}
	}
	if out := DefaultOptions("nope", ""); out != nil {
		t.Fatalf("unknown service: got %v", out)
	}
	if out := DefaultOptions("DB", "reporting"); out == nil {
		t.Fatal("service match must be case-insensitive")
	}
}

func TestDefaultOptionsFresh(t *testing.T) {
	t.Parallel()
	a := DefaultOptions("mailer", "")
	b := DefaultOptions("mailer", "")
	if a != b {
		t.Fatal("fresh defaults must compare equal")
	}
}

func TestResolveFromEnv(t *testing.T) {
	t.Parallel()
	env := Env{"DB_ADAPTER": "postgres", "DB_MAXCONNS": "20", "RATELIMIT_RATE": "2.5"}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
	if cfg.DB.Options.MaxConns != 20 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
	if cfg.RateLimit.Options.Rate != 2.5 {
		t.Fatalf("got %v", cfg.RateLimit.Options.Rate)
	}
}

func TestResolveValidatesFailClosed(t *testing.T) {
	t.Parallel()
	env := Env{"DB_ADAPTER": ""}
	if _, err := Resolve(env); err == nil {
		t.Fatal("want validation error for empty adapter")
	}
}

func TestResolveUnknownFieldFails(t *testing.T) {
	t.Parallel()
	env := Env{"DB_BOGUS_FIELD_XYZ": "1"}
	_, err := Resolve(env)
	if err == nil {
		t.Fatal("want error for unknown field under wired prefix")
	}
	var unknown *UnknownFieldError
	if !errors.As(err, &unknown) {
		t.Fatalf("want UnknownFieldError, got %T: %v", err, err)
	}
}

func TestResolveIgnoresAmbient(t *testing.T) {
	t.Parallel()
	env := Env{"PATH": "/bin", "HOME": "/root", "DB": "x"}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "sqlite" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
}

func FuzzApplyEnvSecretScrub(f *testing.F) {
	for _, seed := range []struct{ key, value string }{
		{"DB_MAXCONNS", "20"},
		{"DB_MAXCONNS", "not-a-number"},
		{"DB_NOPE_X", "x"},
		{"DB_MAXCONBNS", "1"},
		{"DB__R__BOGUS", "v"},
		{"AUTH_JWT_SECRET", "s3cr3t"},
	} {
		f.Add(seed.key, seed.value)
	}
	const canary = "s3cr3t-v4lu3-"
	f.Fuzz(func(t *testing.T, key, value string) {
		t.Parallel()
		if strings.Contains(strings.ToLower(key), canary) {
			return
		}
		cfg := Default()
		err := ApplyEnv(Env{key: canary + value}, "db", "", &cfg.DB)
		if err == nil {
			return
		}
		if strings.Contains(err.Error(), canary) {
			t.Fatalf("ApplyEnv(%q) error echoes the value: %v", key, err)
		}
	})
}
