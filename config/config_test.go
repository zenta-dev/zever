package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// seedHostileEnv sets variables that must never affect Load.
func seedHostileEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AUTH_TOKEN", "hostile")
	t.Setenv("DB", "hostile")
	t.Setenv("USER", "hostile")
	t.Setenv("CONFIGTESTBARE", "hostile")
	t.Setenv("FOO_BAR", "hostile")
}

func TestKnownServiceNames(t *testing.T) {
	names := knownServiceNames()
	if len(names) != 34 {
		t.Fatalf("got %d names, want 34", len(names))
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

func TestLoadExplicitMissing(t *testing.T) {
	seedHostileEnv(t)
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("want error")
	}
	if cfg != nil {
		t.Fatal("want nil config")
	}
}

func TestLoadExplicitUnsupported(t *testing.T) {
	seedHostileEnv(t)
	p := writeFile(t, t.TempDir(), "zever.toml", "x = 1\n")
	_, err := Load(p)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestLoadExplicitBadYAML(t *testing.T) {
	seedHostileEnv(t)
	p := writeFile(t, t.TempDir(), "zever.yaml", ":\tbad:\n")
	if _, err := Load(p); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadExplicitUnknownService(t *testing.T) {
	seedHostileEnv(t)
	p := writeFile(t, t.TempDir(), "zever.yaml", "nope:\n  adapter: x\n")
	_, err := Load(p)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestLoadExplicitUnknownField(t *testing.T) {
	seedHostileEnv(t)
	p := writeFile(t, t.TempDir(), "zever.yaml", "db:\n  adapter: sqlite\n  options:\n    bogus: 1\n")
	if _, err := Load(p); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadDiscoverAbsent(t *testing.T) {
	seedHostileEnv(t)
	t.Chdir(t.TempDir())
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "sqlite" {
		t.Fatalf("got %q, want sqlite", cfg.DB.Adapter)
	}
}

func TestLoadDiscoverYAML(t *testing.T) {
	seedHostileEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "zever.yaml", "db:\n  adapter: postgres\nlog:\n  adapter: noop\n")
	t.Chdir(dir)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
	if cfg.Log.Adapter != "noop" {
		t.Fatalf("got %q", cfg.Log.Adapter)
	}
}

func TestLoadDiscoverJSON(t *testing.T) {
	seedHostileEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "zever.json", `{"db": {"adapter": "postgres"}}`)
	t.Chdir(dir)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
}

func TestLoadDiscoverSkipsDir(t *testing.T) {
	seedHostileEnv(t)
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "zever.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "zever.yml", "db:\n  adapter: postgres\n")
	t.Chdir(dir)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DB.Adapter != "postgres" {
		t.Fatalf("got %q", cfg.DB.Adapter)
	}
}

func TestLoadDiscoverBadFile(t *testing.T) {
	seedHostileEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "zever.yaml", ":\tbad:\n")
	t.Chdir(dir)
	if _, err := Load(""); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadDiscoverMergeError(t *testing.T) {
	seedHostileEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "zever.yaml", "nope:\n  adapter: x\n")
	t.Chdir(dir)
	if _, err := Load(""); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadInvalidFailsNil(t *testing.T) {
	seedHostileEnv(t)
	p := writeFile(t, t.TempDir(), "zever.yaml", "crypto:\n  adapter: local\n  options:\n    key: x\n")
	cfg, err := Load(p)
	if err == nil {
		t.Fatal("want error")
	}
	if cfg != nil {
		t.Fatal("want nil config")
	}
}

func TestLoadEnvTypeError(t *testing.T) {
	seedHostileEnv(t)
	t.Setenv("DB_MAXCONNS", "abc")
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := Load(""); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadEnvBadAdapterFails(t *testing.T) {
	seedHostileEnv(t)
	t.Setenv("DB_ADAPTER", "")
	dir := t.TempDir()
	t.Chdir(dir)
	cfg, err := Load("")
	if err == nil {
		t.Fatal("want error")
	}
	if cfg != nil {
		t.Fatal("want nil config")
	}
}

func TestLoadPrecedence(t *testing.T) {
	seedHostileEnv(t)
	p := writeFile(t, t.TempDir(), "zever.yaml", "db:\n  adapter: postgres\n  options:\n    max_conns: 10\n")
	t.Setenv("DB_MAXCONNS", "20")
	cfg, err := Load(p)
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

func TestRedactedServices(t *testing.T) {
	cfg := Default()
	got := cfg.RedactedServices()
	if len(got) != 34 {
		t.Fatalf("got %d services, want 34", len(got))
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
	t.Setenv("DB_MAXCONNS", secret)
	dir := t.TempDir()
	t.Chdir(dir)
	_, err := Load("")
	if err == nil {
		t.Fatal("want error")
	}
	if got := err.Error(); strings.Contains(got, secret) {
		t.Fatalf("error echoes value: %q", got)
	}
}
