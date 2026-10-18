package config

import (
	"encoding/json"

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
	"github.com/zenta-dev/zever/shared/codec"
)

// Service holds one service's adapter selection and typed options.
// The adapter names the backend implementation (for example "sqlite");
// Options carries that backend's typed settings.
type Service[T any] struct {
	Adapter string ``
	Options T      ``
}

// ServiceConfig is the display envelope for a single service: the adapter
// name plus its redacted option map. It backs RedactedServices, the sole
// display path for option maps.
type ServiceConfig struct {
	// Adapter names the backend implementation (for example "sqlite").
	Adapter string ``
	// Options carries the backend's redacted settings.
	Options map[string]any ``
}

// Config holds the resolved typed configuration for every service.
type Config struct {
	AI            Service[ai.Options]            ``
	Analytics     Service[analytics.Options]     ``
	Auth          Service[auth.Options]          ``
	Billing       Service[billing.Options]       ``
	Cache         Service[cache.Options]         ``
	CDN           Service[cdn.Options]           ``
	Crypto        Service[crypto.Options]        ``
	DB            Service[db.Options]            ``
	Document      Service[document.Options]      ``
	EventBus      Service[eventbus.Options]      ``
	Flag          Service[flag.Options]          ``
	Geo           Service[geo.Options]           ``
	I18n          Service[i18n.Options]          ``
	Idempotency   Service[idempotency.Options]   ``
	Lock          Service[lock.Options]          ``
	Log           Service[log.Options]           ``
	Mailer        Service[mailer.Options]        ``
	Media         Service[media.Options]         ``
	Notification  Service[notification.Options]  ``
	Observability Service[observability.Options] ``
	Outbox        Service[outbox.Options]        ``
	Password      Service[password.Options]      ``
	Payment       Service[payment.Options]       ``
	Permission    Service[permission.Options]    ``
	Queue         Service[queue.Options]         ``
	RateLimit     Service[ratelimit.Options]     ``
	Resilience    Service[resilience.Options]    ``
	Router        Service[router.Options]        ``
	Scheduler     Service[scheduler.Options]     ``
	Search        Service[search.Options]        ``
	Secrets       Service[secrets.Options]       ``
	Session       Service[session.Options]       ``
	Storage       Service[storage.Options]       ``
	Tenant        Service[tenant.Options]        ``
	VectorStore   Service[vectorstore.Options]   ``
	Webhook       Service[webhook.Options]       ``
	Workflow      Service[workflow.Options]      ``
	// Plugins holds third-party plugin services keyed by plugin name. Each
	// entry carries the plugin's adapter selection plus its raw, untyped
	// options: unknown option fields pass through untouched because the
	// plugin validates them itself (see RegisterPluginValidator). Nil by
	// default; Default leaves it nil and merge initializes it on first use.
	Plugins map[string]Service[json.RawMessage] ``
}

// knownServiceNames returns the 37 lowercase service names in sorted order.
// It is the single source of truth for env matching and error paths.
func knownServiceNames() []string {
	return []string{
		"ai", "analytics", "auth", "billing", "cache", "cdn", "crypto", "db",
		"document", "eventbus", "flag", "geo", "i18n", "idempotency",
		"lock", "log", "mailer", "media", "notification", "observability",
		"outbox", "password", "payment", "permission", "queue", "ratelimit",
		"resilience", "router", "scheduler", "search", "secrets", "session",
		"storage", "tenant", "vectorstore", "webhook", "workflow",
	}
}

// mapCodec decodes freshly marshaled JSON into a plain map[string]any; see
// serviceToMap.
var mapCodec = codec.JSONCodec[map[string]any]{}

// serviceToMap converts one service's typed options to a plain map via a
// JSON round-trip, preserving each package's json-tag semantics. It is the
// shared bridge for redaction and env field discovery; reflection for env
// application stays confined to env.go.
func serviceToMap[T any](o T) map[string]any {
	b, err := codec.JSONCodec[T]{}.Encode(o)
	if err != nil {
		return map[string]any{}
	}

	// Decode cannot fail here: b is freshly marshaled JSON produced by the
	// Encode above, so the paired Decode (same underlying encoding/json/v2
	// codec) round-trips it cleanly. We still fall back to an empty map on
	// the (impossible) error path rather than propagate a zero-value nil
	// map, matching the previous json.Marshal/json.Unmarshal behavior.
	m, err := mapCodec.Decode(b)
	if err != nil {
		return map[string]any{}
	}
	return m
}

// RedactedServices returns a per-service view of the resolved config with
// secret values replaced by RedactedValue. It is the sole display path:
// never log raw option maps directly. The returned maps are fresh copies;
// mutating them does not affect the Config.
func (c *Config) RedactedServices() map[string]ServiceConfig {
	out := make(map[string]ServiceConfig, 37+len(c.Plugins))
	// The option maps below are freshly built by serviceToMap and owned
	// by the result, so redaction runs in place: no deep copy is needed
	// to keep the Config untouched.
	put := func(name, adapter string, opts any) {
		m := serviceToMap(opts)
		redactMap(m)
		redactSecretFields(opts, m)
		out[name] = ServiceConfig{Adapter: adapter, Options: m}
	}
	put("ai", c.AI.Adapter, c.AI.Options)
	put("analytics", c.Analytics.Adapter, c.Analytics.Options)
	put("auth", c.Auth.Adapter, c.Auth.Options)
	put("billing", c.Billing.Adapter, c.Billing.Options)
	put("cache", c.Cache.Adapter, c.Cache.Options)
	put("cdn", c.CDN.Adapter, c.CDN.Options)
	put("crypto", c.Crypto.Adapter, c.Crypto.Options)
	put("db", c.DB.Adapter, c.DB.Options)
	put("document", c.Document.Adapter, c.Document.Options)
	put("eventbus", c.EventBus.Adapter, c.EventBus.Options)
	put("flag", c.Flag.Adapter, c.Flag.Options)
	put("geo", c.Geo.Adapter, c.Geo.Options)
	put("i18n", c.I18n.Adapter, c.I18n.Options)
	put("idempotency", c.Idempotency.Adapter, c.Idempotency.Options)
	put("lock", c.Lock.Adapter, c.Lock.Options)
	put("log", c.Log.Adapter, c.Log.Options)
	put("mailer", c.Mailer.Adapter, c.Mailer.Options)
	put("media", c.Media.Adapter, c.Media.Options)
	put("notification", c.Notification.Adapter, c.Notification.Options)
	put("observability", c.Observability.Adapter, c.Observability.Options)
	put("outbox", c.Outbox.Adapter, c.Outbox.Options)
	put("password", c.Password.Adapter, c.Password.Options)
	put("payment", c.Payment.Adapter, c.Payment.Options)
	put("permission", c.Permission.Adapter, c.Permission.Options)
	put("queue", c.Queue.Adapter, c.Queue.Options)
	put("ratelimit", c.RateLimit.Adapter, c.RateLimit.Options)
	put("resilience", c.Resilience.Adapter, c.Resilience.Options)
	put("router", c.Router.Adapter, c.Router.Options)
	put("scheduler", c.Scheduler.Adapter, c.Scheduler.Options)
	put("search", c.Search.Adapter, c.Search.Options)
	put("secrets", c.Secrets.Adapter, c.Secrets.Options)
	put("session", c.Session.Adapter, c.Session.Options)
	put("storage", c.Storage.Adapter, c.Storage.Options)
	put("tenant", c.Tenant.Adapter, c.Tenant.Options)
	put("vectorstore", c.VectorStore.Adapter, c.VectorStore.Options)
	put("webhook", c.Webhook.Adapter, c.Webhook.Options)
	put("workflow", c.Workflow.Adapter, c.Workflow.Options)
	for _, name := range sortedPluginNames(c.Plugins) {
		svc := c.Plugins[name]
		var m map[string]any
		if len(svc.Options) > 0 {
			// Best-effort: corrupt raw options degrade to an empty
			// redacted map rather than failing display.
			if err := json.Unmarshal(svc.Options, &m); err != nil {
				m = nil
			}
		}
		if m == nil {
			m = map[string]any{}
		}
		redactMap(m)
		out[name] = ServiceConfig{Adapter: svc.Adapter, Options: m}
	}
	return out
}
