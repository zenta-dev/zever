package config

import (
	"testing"

	"github.com/zenta-dev/zever/password"
)

func TestDefaultSnapshot(t *testing.T) {
	t.Parallel()
	cfg := Default()
	want := map[string]string{
		"ai": "anthropic", "analytics": "log", "auth": "jwt", "billing": "stub",
		"cache": "memory", "crypto": "local", "db": "sqlite", "document": "local",
		"eventbus": "memory", "flag": "static", "geo": "static", "i18n": "embed",
		"idempotency": "memory", "log": "slog", "mailer": "log", "media": "local",
		"notification": "log", "observability": "stdout", "password": "argon2id",
		"payment": "stub", "permission": "noop", "queue": "memory", "ratelimit": "memory",
		"router": "stdhttp", "scheduler": "embedded", "search": "sqlite", "session": "memory",
		"storage": "local", "tenant": "single", "vectorstore": "sqlite", "webhook": "http",
		"workflow": "memory",
	}
	got := map[string]string{
		"ai": cfg.AI.Adapter, "analytics": cfg.Analytics.Adapter, "auth": cfg.Auth.Adapter,
		"billing": cfg.Billing.Adapter, "cache": cfg.Cache.Adapter, "crypto": cfg.Crypto.Adapter,
		"db": cfg.DB.Adapter, "document": cfg.Document.Adapter, "eventbus": cfg.Eventbus.Adapter,
		"flag": cfg.Flag.Adapter, "geo": cfg.Geo.Adapter, "i18n": cfg.I18n.Adapter,
		"idempotency": cfg.Idempotency.Adapter, "log": cfg.Log.Adapter, "mailer": cfg.Mailer.Adapter,
		"media": cfg.Media.Adapter, "notification": cfg.Notification.Adapter,
		"observability": cfg.Observability.Adapter, "password": cfg.Password.Adapter,
		"payment": cfg.Payment.Adapter, "permission": cfg.Permission.Adapter,
		"queue": cfg.Queue.Adapter, "ratelimit": cfg.Ratelimit.Adapter, "router": cfg.Router.Adapter,
		"scheduler": cfg.Scheduler.Adapter, "search": cfg.Search.Adapter,
		"session": cfg.Session.Adapter, "storage": cfg.Storage.Adapter, "tenant": cfg.Tenant.Adapter,
		"vectorstore": cfg.VectorStore.Adapter, "webhook": cfg.Webhook.Adapter,
		"workflow": cfg.Workflow.Adapter,
	}
	for svc, w := range want {
		if got[svc] != w {
			t.Errorf("%s: got %q, want %q", svc, got[svc], w)
		}
	}
	if cfg.Crypto.Options.Key == "" {
		t.Error("crypto dev key missing")
	}
	if cfg.Mailer.Options.Host == "" || cfg.Mailer.Options.Port == 0 {
		t.Error("mailer dummy host/port missing")
	}
	if cfg.Observability.Options.ServiceName == "" {
		t.Error("observability service name missing")
	}
	if cfg.Password.Options.Time != password.DefaultTime ||
		cfg.Password.Options.Memory != password.DefaultMemory ||
		cfg.Password.Options.Threads != password.DefaultThreads ||
		cfg.Password.Options.SaltLen != password.DefaultSaltLen ||
		cfg.Password.Options.KeyLen != password.DefaultKeyLen {
		t.Error("password defaults mismatch")
	}
	if cfg.Ratelimit.Options.Rate <= 0 || cfg.Ratelimit.Options.Burst <= 0 {
		t.Error("ratelimit defaults must be positive")
	}
	if cfg.Scheduler.Options.Dispatcher == nil {
		t.Error("scheduler dispatcher missing")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default must pass Validate: %v", err)
	}
}
