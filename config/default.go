package config

import (
	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/analytics"
	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/billing"
	"github.com/zenta-dev/zever/cache"
	"github.com/zenta-dev/zever/crypto"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/document"
	"github.com/zenta-dev/zever/eventbus"
	"github.com/zenta-dev/zever/flag"
	"github.com/zenta-dev/zever/geo"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/lock"
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/mailer"
	"github.com/zenta-dev/zever/media"
	"github.com/zenta-dev/zever/notification"
	"github.com/zenta-dev/zever/observability"
	"github.com/zenta-dev/zever/password"
	"github.com/zenta-dev/zever/payment"
	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/queue"
	"github.com/zenta-dev/zever/ratelimit"
	"github.com/zenta-dev/zever/router"
	"github.com/zenta-dev/zever/scheduler"
	"github.com/zenta-dev/zever/search"
	"github.com/zenta-dev/zever/secrets"
	"github.com/zenta-dev/zever/session"
	"github.com/zenta-dev/zever/storage"
	"github.com/zenta-dev/zever/tenant"
	"github.com/zenta-dev/zever/vectorstore"
	"github.com/zenta-dev/zever/webhook"
	"github.com/zenta-dev/zever/workflow"
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
