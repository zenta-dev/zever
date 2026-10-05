package app

import (
	"os"
	"strings"
	"testing"
)

// TestConfigDefaultsAndJWT pins the resolved-config contract: the sqlite
// path is filled with DefaultDBPath, the embedded i18n catalog and RBAC
// rules are injected, and the JWT secret fails closed when missing or too
// short.
func TestConfigDefaultsAndJWT(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AUTH_JWT_SECRET", strings.Repeat("x", 32))

	cfg, err := Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}

	if cfg.DB.Options.Path != DefaultDBPath {
		t.Fatalf("DB path = %q, want %q", cfg.DB.Options.Path, DefaultDBPath)
	}

	if cfg.I18n.Options.Embed.FS == nil {
		t.Fatal("i18n embed FS is nil, want the embedded catalog")
	}

	if len(cfg.Permission.Options.Rules) != 2 {
		t.Fatalf("permission rules = %d, want 2", len(cfg.Permission.Options.Rules))
	}

	if cfg.Auth.Options.JWT.Secret != strings.Repeat("x", 32) {
		t.Fatalf("JWT secret = %q, want the AUTH_JWT_SECRET value", cfg.Auth.Options.JWT.Secret)
	}
}

// TestConfigFailsClosedWithoutJWTSecret proves Config rejects a missing or
// too-short AUTH_JWT_SECRET instead of booting with an insecure key.
func TestConfigFailsClosedWithoutJWTSecret(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AUTH_JWT_SECRET", "short")

	_, err := Config()
	if err == nil {
		t.Fatal("Config with short AUTH_JWT_SECRET = nil error, want fail-closed error")
	}

	if !strings.Contains(err.Error(), "AUTH_JWT_SECRET") {
		t.Fatalf("error = %q, want it to name AUTH_JWT_SECRET", err.Error())
	}
}

// TestNewResolvesDB proves the container New builds resolves the database
// battery against the defaulted sqlite path.
func TestNewResolvesDB(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AUTH_JWT_SECRET", strings.Repeat("x", 32))

	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := t.Context()

	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}

	if err := database.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
