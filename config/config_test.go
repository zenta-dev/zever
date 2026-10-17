package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// seedHostileEnv sets process variables that must never affect Resolve:
// unknown heads and bare names are ignored. AUTH_TOKEN is deliberately
// absent: AUTH is a wired service head, so it fails fast under the unknown-
// key rule (pinned by TestEnvUnknownFieldFails).
func seedHostileEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB", "hostile")
	t.Setenv("USER", "hostile")
	t.Setenv("CONFIGTESTBARE", "hostile")
	t.Setenv("FOO_BAR", "hostile")
}

func TestKnownServiceNames(t *testing.T) {
	names := knownServiceNames()
	if len(names) != 37 {
		t.Fatalf("got %d names, want 37", len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("names not sorted: %q >= %q", names[i-1], names[i])
		}
	}
	for _, want := range []string{"ai", "db", "lock", "secrets", "vectorstore", "workflow"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing service %q", want)
		}
	}
}

func TestResolveDefaults(t *testing.T) {
	seedHostileEnv(t)
	cfg, err := Resolve(Env{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "sqlite" {
		t.Fatalf("got %q, want sqlite", cfg.DB.Adapter)
	}
}

func TestResolveEnvOverrides(t *testing.T) {
	seedHostileEnv(t)
	cfg, err := Resolve(Env{"DB_ADAPTER": "postgres", "DB_MAXCONNS": "20"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
	if cfg.DB.Options.MaxConns != 20 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
}

func TestResolveInvalidFailsNil(t *testing.T) {
	seedHostileEnv(t)
	cfg, err := Resolve(Env{"CRYPTO_ADAPTER": "local", "CRYPTO_KEY": "x"})
	if err == nil {
		t.Fatal("want error")
	}
	if cfg != nil {
		t.Fatal("want nil config")
	}
}

func TestResolveEnvTypeError(t *testing.T) {
	seedHostileEnv(t)
	if _, err := Resolve(Env{"DB_MAXCONNS": "abc"}); err == nil {
		t.Fatal("want error")
	}
}

func TestResolveEnvBadAdapterFails(t *testing.T) {
	seedHostileEnv(t)
	cfg, err := Resolve(Env{"DB_ADAPTER": ""})
	if err == nil {
		t.Fatal("want error")
	}
	if cfg != nil {
		t.Fatal("want nil config")
	}
}

func TestFromEnvReadsDotEnv(t *testing.T) {
	seedHostileEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, ".env", "DB_ADAPTER=postgres\n")
	t.Chdir(dir)
	env, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if env["DB_ADAPTER"] != "postgres" {
		t.Fatalf("got %q", env["DB_ADAPTER"])
	}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
}

func TestFromEnvMalformedDotEnvFails(t *testing.T) {
	seedHostileEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, ".env", ":\tbad:\n")
	t.Chdir(dir)
	if _, err := FromEnv(); err == nil {
		t.Fatal("want error")
	}
}

func TestRedactedServices(t *testing.T) {
	cfg := Default()
	got := cfg.RedactedServices()
	if len(got) != 37 {
		t.Fatalf("got %d services, want 37", len(got))
	}
	cryptoSvc := got["crypto"]
	if cryptoSvc.Adapter != "local" {
		t.Fatalf("got %q", cryptoSvc.Adapter)
	}
	if cryptoSvc.Options["key"] != RedactedValue {
		t.Fatalf("key not redacted: %v", cryptoSvc.Options["key"])
	}
	if _, ok := got["eventbus"]; !ok {
		t.Fatal("missing eventbus")
	}
	got["crypto"].Options["key"] = "mutated"
	if cfg.Crypto.Options.Key == "mutated" || cfg.Crypto.Options.Key == "" {
		t.Fatal("mutation leaked into config")
	}
	fresh := cfg.RedactedServices()
	if fresh["crypto"].Options["key"] != RedactedValue {
		t.Fatal("not non-mutating")
	}
}

func TestErrorSecretsNeverInText(t *testing.T) {
	seedHostileEnv(t)
	secret := "super-secret-value-xyz-123"
	_, err := Resolve(Env{"DB_MAXCONNS": secret})
	if err == nil {
		t.Fatal("want error")
	}
	if got := err.Error(); strings.Contains(got, secret) {
		t.Fatalf("error echoes value: %q", got)
	}
}
