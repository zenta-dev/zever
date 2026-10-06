package config

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkDefault measures building the zero-infrastructure default config.
func BenchmarkDefault(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if Default() == nil {
			b.Fatal("nil config")
		}
	}
}

// BenchmarkLoadFile measures the full layered load (decode, strict merge,
// best-effort env overlay, fail-closed validate) from a small valid file.
func BenchmarkLoadFile(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "zever.yaml")
	if err := os.WriteFile(path, []byte("db:\n  adapter: sqlite\n  options:\n    max_conns: 10\nlog:\n  adapter: slog\n"), 0o600); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cfg, err := Load(path)
		if err != nil {
			b.Fatalf("Load: %v", err)
		}
		if cfg.DB.Options.MaxConns != 10 {
			b.Fatalf("max_conns = %d, want 10", cfg.DB.Options.MaxConns)
		}
	}
}

// BenchmarkValidate measures the fail-closed validation sweep across all 36
// services plus plugins.
func BenchmarkValidate(b *testing.B) {
	cfg := Default()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := cfg.Validate(); err != nil {
			b.Fatalf("Validate: %v", err)
		}
	}
}

// BenchmarkRedactedServices measures building the redacted per-service view
// used by every display path.
func BenchmarkRedactedServices(b *testing.B) {
	cfg := Default()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := cfg.RedactedServices(); len(got) != 37 {
			b.Fatalf("got %d services, want 37", len(got))
		}
	}
}

// BenchmarkRedact measures the recursive redaction of a nested option map.
func BenchmarkRedact(b *testing.B) {
	m := map[string]any{
		"dsn":    "postgres://user@localhost/db",
		"nested": map[string]any{"api_key": "abc", "keep": "ok"},
		"list":   []any{map[string]any{"token": "xyz"}, "plain"},
		"headers": []string{
			"Authorization: Bearer abc",
			"Content-Type: application/json",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out := Redact(m)
		if out["dsn"] != RedactedValue {
			b.Fatalf("dsn not redacted: %v", out["dsn"])
		}
	}
}

// BenchmarkApplyEnv measures the best-effort environment overlay, the only
// reflection-bearing path in config.
func BenchmarkApplyEnv(b *testing.B) {
	b.Setenv("DB_MAXCONNS", "20")
	cfg := Default()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := applyEnv(cfg); err != nil {
			b.Fatalf("applyEnv: %v", err)
		}
	}
}
