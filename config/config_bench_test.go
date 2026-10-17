package config

import (
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

// BenchmarkResolveEnv measures the full env-only resolution (defaults,
// env overlay, fail-closed validate) from a small valid Env.
func BenchmarkResolveEnv(b *testing.B) {
	env := Env{"DB_ADAPTER": "sqlite", "DB_MAXCONNS": "10", "LOG_ADAPTER": "slog"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cfg, err := Resolve(env)
		if err != nil {
			b.Fatalf("Resolve: %v", err)
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

// BenchmarkApplyEnv measures the environment overlay, the only
// reflection-bearing path in config.
func BenchmarkApplyEnv(b *testing.B) {
	env := Env{"DB_MAXCONNS": "20"}
	cfg := Default()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := ApplyEnv(env, "db", "", &cfg.DB); err != nil {
			b.Fatalf("ApplyEnv: %v", err)
		}
	}
}
