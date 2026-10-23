package config

import (
	"errors"
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
	if cfg.Crypto.Options.Key == "" {
		t.Fatal("want default crypto dev key")
	}
}

func TestResolveEnvOverrides(t *testing.T) {
	seedHostileEnv(t)
	cfg, err := Resolve(Env{"DB_MAXCONNS": "20"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 20 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
	}
}

func TestResolveInvalidFailsNil(t *testing.T) {
	seedHostileEnv(t)
	cfg, err := Resolve(Env{"CRYPTO_KEY": "x"})
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

// TestResolveEnvAdapterRejected pins the removal: any *_ADAPTER key fails
// resolution with an UnknownFieldError carrying the removal message —
// never silently ignored, never a did-you-mean suggestion. On failure
// Resolve returns nil.
func TestResolveEnvAdapterRejected(t *testing.T) {
	seedHostileEnv(t)
	for _, key := range []string{
		"DB_ADAPTER",
		"db_adapter",
		"DB__REPORTING__ADAPTER",
		"FOO_ADAPTER",
		"FOO_BAR_ADAPTER",
	} {
		t.Run(key, func(t *testing.T) {
			cfg, err := Resolve(Env{key: "sqlite"})
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
			if !errors.Is(err, ErrUnknownField) {
				t.Fatalf("%s: want ErrUnknownField match: %v", key, err)
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

func TestFromEnvReadsDotEnv(t *testing.T) {
	seedHostileEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, ".env", "DB_MAXCONNS=20\n")
	t.Chdir(dir)
	env, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if env["DB_MAXCONNS"] != "20" {
		t.Fatalf("got %q", env["DB_MAXCONNS"])
	}
	cfg, err := Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Options.MaxConns != 20 {
		t.Fatalf("got %d", cfg.DB.Options.MaxConns)
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
