package container

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

// registeredAdapters maps a config service name to its battery's registry
// listing. It is the process-local source of truth handed to
// config.ValidateAdapters: an adapter counts as known only when a factory
// was registered for it in this process (blank or direct adapter imports do
// that). Unknown service names yield no registrations.
func registeredAdapters(service string) []string {
	switch service {
	case "ai":
		return ai.RegisteredAdapters()
	case "analytics":
		return analytics.RegisteredAdapters()
	case "auth":
		return auth.RegisteredAdapters()
	case "billing":
		return billing.RegisteredAdapters()
	case "cache":
		return cache.RegisteredAdapters()
	case "cdn":
		return cdn.RegisteredAdapters()
	case "crypto":
		return crypto.RegisteredAdapters()
	case "db":
		return db.RegisteredAdapters()
	case "document":
		return document.RegisteredAdapters()
	case "eventbus":
		return eventbus.RegisteredAdapters()
	case "flag":
		return flag.RegisteredAdapters()
	case "geo":
		return geo.RegisteredAdapters()
	case "i18n":
		return i18n.RegisteredAdapters()
	case "idempotency":
		return idempotency.RegisteredAdapters()
	case "lock":
		return lock.RegisteredAdapters()
	case "log":
		return log.RegisteredAdapters()
	case "mailer":
		return mailer.RegisteredAdapters()
	case "media":
		return media.RegisteredAdapters()
	case "notification":
		return notification.RegisteredAdapters()
	case "observability":
		return observability.RegisteredAdapters()
	case "outbox":
		return outbox.RegisteredAdapters()
	case "password":
		return password.RegisteredAdapters()
	case "payment":
		return payment.RegisteredAdapters()
	case "permission":
		return permission.RegisteredAdapters()
	case "queue":
		return queue.RegisteredAdapters()
	case "ratelimit":
		return ratelimit.RegisteredAdapters()
	case "resilience":
		return resilience.RegisteredAdapters()
	case "router":
		return router.RegisteredAdapters()
	case "scheduler":
		return scheduler.RegisteredAdapters()
	case "search":
		return search.RegisteredAdapters()
	case "secrets":
		return secrets.RegisteredAdapters()
	case "session":
		return session.RegisteredAdapters()
	case "storage":
		return storage.RegisteredAdapters()
	case "tenant":
		return tenant.RegisteredAdapters()
	case "vectorstore":
		return vectorstore.RegisteredAdapters()
	case "webhook":
		return webhook.RegisteredAdapters()
	case "workflow":
		return workflow.RegisteredAdapters()
	default:
		return nil
	}
}

// RegisteredAdapters returns the sorted names of adapters with a registered
// factory for service, or nil when the service is unknown. It is the
// `registered` callback config.ValidateAdapters expects: pass it directly to
// validate a config against this process's registrations.
func RegisteredAdapters(service string) []string {
	return registeredAdapters(service)
}

// ValidateAdapters checks the container's config against this process's
// adapter registrations and fails fast on unknown adapters, with a "did you
// mean %q?" hint when a registered name is close. Call it right after New
// (or from `zever doctor`) to turn a forgotten adapter import into one
// actionable error instead of per-service resolve failures. New itself stays
// infallible: nothing opens until first use.
func (c *Container) ValidateAdapters() error {
	return c.cfg.ValidateAdapters(registeredAdapters)
}
