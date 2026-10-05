package app

import (
	"os"
	"strings"
	"testing"
)

// TestConfigFillsDemoDefaults pins every demo-only default Config injects
// when the corresponding option is unset: file paths, RBAC rules, the
// embedded i18n catalog, and the vector dimension.
func TestConfigFillsDemoDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AUTH_JWT_SECRET", "")

	cfg, err := Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}

	if cfg.DB.Options.Path != DefaultDBPath {
		t.Fatalf("DB path = %q, want %q", cfg.DB.Options.Path, DefaultDBPath)
	}

	if cfg.Auth.Options.JWT.Secret != DevJWTSecret {
		t.Fatalf("JWT secret = %q, want DevJWTSecret", cfg.Auth.Options.JWT.Secret)
	}

	if cfg.Flag.Options.Static.Path != DefaultFlagsPath {
		t.Fatalf("flag path = %q, want %q", cfg.Flag.Options.Static.Path, DefaultFlagsPath)
	}

	if len(cfg.Permission.Options.Rules) != 5 {
		t.Fatalf("permission rules = %d, want 5", len(cfg.Permission.Options.Rules))
	}

	if cfg.I18n.Options.Embed.FS == nil {
		t.Fatal("i18n embed FS is nil, want the embedded catalog")
	}

	if cfg.I18n.Options.Embed.Dir != "." || cfg.I18n.Options.Embed.Fallback != "en" {
		t.Fatalf("i18n embed = %+v, want dir . fallback en", cfg.I18n.Options.Embed)
	}

	if cfg.Storage.Options.Root != DefaultStorageRoot {
		t.Fatalf("storage root = %q, want %q", cfg.Storage.Options.Root, DefaultStorageRoot)
	}

	if cfg.Media.Options.Root != DefaultMediaRoot {
		t.Fatalf("media root = %q, want %q", cfg.Media.Options.Root, DefaultMediaRoot)
	}

	if cfg.Search.Options.DSN != DefaultSearchDSN {
		t.Fatalf("search DSN = %q, want %q", cfg.Search.Options.DSN, DefaultSearchDSN)
	}

	if cfg.VectorStore.Options.DSN != DefaultVectorDSN {
		t.Fatalf("vector DSN = %q, want %q", cfg.VectorStore.Options.DSN, DefaultVectorDSN)
	}

	if cfg.VectorStore.Options.Dimension != VectorDims {
		t.Fatalf("vector dimension = %d, want %d", cfg.VectorStore.Options.Dimension, VectorDims)
	}

	if cfg.Geo.Options.Path != DefaultCitiesPath {
		t.Fatalf("geo path = %q, want %q", cfg.Geo.Options.Path, DefaultCitiesPath)
	}
}

// TestConfigEnvOverrideWins proves environment variables beat the demo
// defaults: AUTH_JWT_SECRET replaces DevJWTSecret and DB_PATH replaces
// DefaultDBPath.
func TestConfigEnvOverrideWins(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AUTH_JWT_SECRET", strings.Repeat("z", 40))

	overrideDB := t.TempDir() + "/override.db"
	t.Setenv("DB_PATH", overrideDB)

	cfg, err := Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}

	if cfg.Auth.Options.JWT.Secret != strings.Repeat("z", 40) {
		t.Fatalf("JWT secret = %q, want the AUTH_JWT_SECRET value", cfg.Auth.Options.JWT.Secret)
	}

	if cfg.DB.Options.Path != overrideDB {
		t.Fatalf("DB path = %q, want the DB_PATH value", cfg.DB.Options.Path)
	}
}

// TestNewResolvesDB proves the container New builds resolves the database
// battery against the defaulted sqlite path.
func TestNewResolvesDB(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AUTH_JWT_SECRET", strings.Repeat("y", 32))

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
