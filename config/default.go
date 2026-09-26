package config

import (
	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/media"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/core/workflow"
)

// Default returns zero-infrastructure adapters so tests run without manual
// setup. Deterministic dev secrets appear ONLY where Validate requires
// them (crypto key); everything else stays zero-valued.
//
// WARNING: dev secrets are deterministic and public. Production must supply
// real secrets via file or environment — never ship defaults.
func Default() *Config {
	cfg := &Config{}
	cfg.AI = Service[ai.Options]{Adapter: "anthropic"}
	cfg.Analytics = Service[analytics.Options]{Adapter: "log"}
	cfg.Auth = Service[auth.Options]{Adapter: "jwt"}
	cfg.Billing = Service[billing.Options]{Adapter: "stub"}
	cfg.Cache = Service[cache.Options]{Adapter: "memory"}
	cfg.Crypto = Service[crypto.Options]{Adapter: "local", Options: crypto.Options{Key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="}} //nolint:gosec // deterministic dev-only secret, never production
	cfg.DB = Service[db.Options]{Adapter: "sqlite"}
	cfg.Document = Service[document.Options]{Adapter: "local"}
	cfg.EventBus = Service[eventbus.Options]{Adapter: "memory"}
	cfg.Flag = Service[flag.Options]{Adapter: "static"}
	cfg.Geo = Service[geo.Options]{Adapter: "static"}
	cfg.I18n = Service[i18n.Options]{Adapter: "embed"}
	cfg.Idempotency = Service[idempotency.Options]{Adapter: "memory"}
	cfg.Lock = Service[lock.Options]{Adapter: "memory"}
	cfg.Log = Service[log.Options]{Adapter: "slog"}
	cfg.Mailer = Service[mailer.Options]{Adapter: "log", Options: mailer.Options{Host: "localhost", Port: 25}}
	cfg.Media = Service[media.Options]{Adapter: "local"}
	cfg.Notification = Service[notification.Options]{Adapter: "log"}
	cfg.Observability = Service[observability.Options]{Adapter: "stdout", Options: observability.Options{ServiceName: "zever"}}
	cfg.Password = Service[password.Options]{Adapter: "argon2id", Options: password.Options{Time: password.DefaultTime, Memory: password.DefaultMemory, Threads: password.DefaultThreads, SaltLen: password.DefaultSaltLen, KeyLen: password.DefaultKeyLen}}
	// WARNING: stub payment performs zero webhook verification (fail-closed)
	// and holds no real funds. Test-only, never production.
	cfg.Payment = Service[payment.Options]{Adapter: "stub"}
	cfg.Permission = Service[permission.Options]{Adapter: "noop"}
	cfg.Queue = Service[queue.Options]{Adapter: "memory"}
	cfg.RateLimit = Service[ratelimit.Options]{Adapter: "memory", Options: ratelimit.Options{Rate: 10, Burst: 20}}
	cfg.Router = Service[router.Options]{Adapter: "stdhttp"}
	cfg.Scheduler = Service[scheduler.Options]{Adapter: "embedded", Options: scheduler.Options{Dispatcher: &job.Dispatcher{}}}
	cfg.Search = Service[search.Options]{Adapter: "sqlite"}
	cfg.Secrets = Service[secrets.Options]{Adapter: "env", Options: secrets.Options{Prefix: "ZEVER"}}
	cfg.Session = Service[session.Options]{Adapter: "memory"}
	cfg.Storage = Service[storage.Options]{Adapter: "local"}
	cfg.Tenant = Service[tenant.Options]{Adapter: "single"}
	cfg.VectorStore = Service[vectorstore.Options]{Adapter: "sqlite"}
	cfg.Webhook = Service[webhook.Options]{Adapter: "http"}
	cfg.Workflow = Service[workflow.Options]{Adapter: "memory"}
	return cfg
}
