package config

import (
	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/cdn"
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
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/resilience"
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

// Default returns options-only zero-infrastructure configuration so tests
// run without manual setup. Backend selection is compile-time
// (container.Provide plus direct imports), never environment: Default
// carries only option values. Deterministic dev secrets appear ONLY where
// Validate requires them (crypto key); everything else stays zero-valued.
//
// WARNING: dev secrets are deterministic and public. Production must supply
// real secrets via file or environment — never ship defaults.
func Default() *Config {
	cfg := &Config{}
	cfg.AI = Service[ai.Options]{}
	cfg.Analytics = Service[analytics.Options]{}
	cfg.Auth = Service[auth.Options]{}
	cfg.Billing = Service[billing.Options]{}
	cfg.Cache = Service[cache.Options]{}
	cfg.CDN = Service[cdn.Options]{}
	cfg.Crypto = Service[crypto.Options]{Options: crypto.Options{Key: crypto.DevCryptoKey}} // deterministic dev-only secret, never production
	cfg.DB = Service[db.Options]{}
	cfg.Document = Service[document.Options]{}
	cfg.EventBus = Service[eventbus.Options]{}
	cfg.Flag = Service[flag.Options]{}
	cfg.Geo = Service[geo.Options]{}
	cfg.I18n = Service[i18n.Options]{}
	cfg.Idempotency = Service[idempotency.Options]{}
	cfg.Lock = Service[lock.Options]{}
	cfg.Log = Service[log.Options]{}
	cfg.Mailer = Service[mailer.Options]{Options: mailer.Options{Host: "localhost", Port: 25}}
	cfg.Media = Service[media.Options]{}
	cfg.Notification = Service[notification.Options]{}
	cfg.Observability = Service[observability.Options]{Options: observability.Options{ServiceName: "zever"}}
	cfg.Outbox = Service[outbox.Options]{}
	cfg.Password = Service[password.Options]{Options: password.Options{Time: password.DefaultTime, Memory: password.DefaultMemory, Threads: password.DefaultThreads, SaltLen: password.DefaultSaltLen, KeyLen: password.DefaultKeyLen}}
	// WARNING: stub payment performs zero webhook verification (fail-closed)
	// and holds no real funds. Test-only, never production.
	cfg.Payment = Service[payment.Options]{}
	cfg.Permission = Service[permission.Options]{}
	cfg.Queue = Service[queue.Options]{}
	cfg.RateLimit = Service[ratelimit.Options]{Options: ratelimit.Options{Rate: 10, Burst: 20}}
	cfg.Resilience = Service[resilience.Options]{}
	cfg.Router = Service[router.Options]{}
	cfg.Scheduler = Service[scheduler.Options]{Options: scheduler.Options{Dispatcher: &job.Dispatcher{}}}
	cfg.Search = Service[search.Options]{}
	cfg.Secrets = Service[secrets.Options]{Options: secrets.Options{Prefix: "ZEVER"}}
	cfg.Session = Service[session.Options]{}
	cfg.Storage = Service[storage.Options]{}
	cfg.Tenant = Service[tenant.Options]{}
	cfg.VectorStore = Service[vectorstore.Options]{}
	cfg.Webhook = Service[webhook.Options]{}
	cfg.Workflow = Service[workflow.Options]{}
	return cfg
}
