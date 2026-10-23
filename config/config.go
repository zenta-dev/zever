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

// Service holds one service's typed options. Backend selection is
// compile-time (container.Provide plus direct imports); the environment
// only tunes options, never selects backends.
type Service[T any] struct {
	Options T `json:"options"`
}

// ServiceConfig is the display envelope for a single service: its redacted
// option map. It backs RedactedServices, the sole display path for option
// maps.
type ServiceConfig struct {
	// Options carries the backend's redacted settings.
	Options map[string]any `json:"options"`
}

// Config holds the resolved typed configuration for every service.
type Config struct {
	AI            Service[ai.Options]            `json:"ai"`
	Analytics     Service[analytics.Options]     `json:"analytics"`
	Auth          Service[auth.Options]          `json:"auth"`
	Billing       Service[billing.Options]       `json:"billing"`
	Cache         Service[cache.Options]         `json:"cache"`
	CDN           Service[cdn.Options]           `json:"cdn"`
	Crypto        Service[crypto.Options]        `json:"crypto"`
	DB            Service[db.Options]            `json:"db"`
	Document      Service[document.Options]      `json:"document"`
	EventBus      Service[eventbus.Options]      `json:"eventbus"`
	Flag          Service[flag.Options]          `json:"flag"`
	Geo           Service[geo.Options]           `json:"geo"`
	I18n          Service[i18n.Options]          `json:"i18n"`
	Idempotency   Service[idempotency.Options]   `json:"idempotency"`
	Lock          Service[lock.Options]          `json:"lock"`
	Log           Service[log.Options]           `json:"log"`
	Mailer        Service[mailer.Options]        `json:"mailer"`
	Media         Service[media.Options]         `json:"media"`
	Notification  Service[notification.Options]  `json:"notification"`
	Observability Service[observability.Options] `json:"observability"`
	Outbox        Service[outbox.Options]        `json:"outbox"`
	Password      Service[password.Options]      `json:"password"`
	Payment       Service[payment.Options]       `json:"payment"`
	Permission    Service[permission.Options]    `json:"permission"`
	Queue         Service[queue.Options]         `json:"queue"`
	RateLimit     Service[ratelimit.Options]     `json:"ratelimit"`
	Resilience    Service[resilience.Options]    `json:"resilience"`
	Router        Service[router.Options]        `json:"router"`
	Scheduler     Service[scheduler.Options]     `json:"scheduler"`
	Search        Service[search.Options]        `json:"search"`
	Secrets       Service[secrets.Options]       `json:"secrets"`
	Session       Service[session.Options]       `json:"session"`
	Storage       Service[storage.Options]       `json:"storage"`
	Tenant        Service[tenant.Options]        `json:"tenant"`
	VectorStore   Service[vectorstore.Options]   `json:"vectorstore"`
	Webhook       Service[webhook.Options]       `json:"webhook"`
	Workflow      Service[workflow.Options]      `json:"workflow"`
	// Plugins holds third-party plugin services keyed by plugin name. Each
	// entry carries its raw, untyped options: unknown option fields pass
	// through untouched because the plugin validates them itself (see
	// RegisterPluginValidator). Nil by default; Default leaves it nil and
	// merge initializes it on first use.
	Plugins map[string]Service[json.RawMessage] `json:"plugins"`
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
	put := func(name string, opts any) {
		m := serviceToMap(opts)
		redactMap(m)
		redactSecretFields(opts, m)
		out[name] = ServiceConfig{Options: m}
	}
	put("ai", c.AI.Options)
	put("analytics", c.Analytics.Options)
	put("auth", c.Auth.Options)
	put("billing", c.Billing.Options)
	put("cache", c.Cache.Options)
	put("cdn", c.CDN.Options)
	put("crypto", c.Crypto.Options)
	put("db", c.DB.Options)
	put("document", c.Document.Options)
	put("eventbus", c.EventBus.Options)
	put("flag", c.Flag.Options)
	put("geo", c.Geo.Options)
	put("i18n", c.I18n.Options)
	put("idempotency", c.Idempotency.Options)
	put("lock", c.Lock.Options)
	put("log", c.Log.Options)
	put("mailer", c.Mailer.Options)
	put("media", c.Media.Options)
	put("notification", c.Notification.Options)
	put("observability", c.Observability.Options)
	put("outbox", c.Outbox.Options)
	put("password", c.Password.Options)
	put("payment", c.Payment.Options)
	put("permission", c.Permission.Options)
	put("queue", c.Queue.Options)
	put("ratelimit", c.RateLimit.Options)
	put("resilience", c.Resilience.Options)
	put("router", c.Router.Options)
	put("scheduler", c.Scheduler.Options)
	put("search", c.Search.Options)
	put("secrets", c.Secrets.Options)
	put("session", c.Session.Options)
	put("storage", c.Storage.Options)
	put("tenant", c.Tenant.Options)
	put("vectorstore", c.VectorStore.Options)
	put("webhook", c.Webhook.Options)
	put("workflow", c.Workflow.Options)
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
		out[name] = ServiceConfig{Options: m}
	}
	return out
}
