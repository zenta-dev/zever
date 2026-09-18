package config

import (
	"os"

	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/analytics"
	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/billing"
	"github.com/zenta-dev/zever/cache"
	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/crypto"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/document"
	"github.com/zenta-dev/zever/eventbus"
	"github.com/zenta-dev/zever/flag"
	"github.com/zenta-dev/zever/geo"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/idempotency"
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

// Service holds one service's adapter selection and typed options.
// The adapter names the backend implementation (for example "sqlite");
// Options carries that backend's typed settings.
type Service[T any] struct {
	Adapter string `json:"adapter" yaml:"adapter"`
	Options T      `json:"options" yaml:"options"`
}

// Config holds the resolved typed configuration for every service.
type Config struct {
	AI            Service[ai.Options]            `json:"ai" yaml:"ai"`
	Analytics     Service[analytics.Options]     `json:"analytics" yaml:"analytics"`
	Auth          Service[auth.Options]          `json:"auth" yaml:"auth"`
	Billing       Service[billing.Options]       `json:"billing" yaml:"billing"`
	Cache         Service[cache.Options]         `json:"cache" yaml:"cache"`
	Crypto        Service[crypto.Options]        `json:"crypto" yaml:"crypto"`
	DB            Service[db.Options]            `json:"db" yaml:"db"`
	Document      Service[document.Options]      `json:"document" yaml:"document"`
	Eventbus      Service[eventbus.Options]      `json:"eventbus" yaml:"eventbus"`
	Flag          Service[flag.Options]          `json:"flag" yaml:"flag"`
	Geo           Service[geo.Options]           `json:"geo" yaml:"geo"`
	I18n          Service[i18n.Options]          `json:"i18n" yaml:"i18n"`
	Idempotency   Service[idempotency.Options]   `json:"idempotency" yaml:"idempotency"`
	Lock          Service[lock.Options]          `json:"lock" yaml:"lock"`
	Log           Service[log.Options]           `json:"log" yaml:"log"`
	Mailer        Service[mailer.Options]        `json:"mailer" yaml:"mailer"`
	Media         Service[media.Options]         `json:"media" yaml:"media"`
	Notification  Service[notification.Options]  `json:"notification" yaml:"notification"`
	Observability Service[observability.Options] `json:"observability" yaml:"observability"`
	Password      Service[password.Options]      `json:"password" yaml:"password"`
	Payment       Service[payment.Options]       `json:"payment" yaml:"payment"`
	Permission    Service[permission.Options]    `json:"permission" yaml:"permission"`
	Queue         Service[queue.Options]         `json:"queue" yaml:"queue"`
	Ratelimit     Service[ratelimit.Options]     `json:"ratelimit" yaml:"ratelimit"`
	Router        Service[router.Options]        `json:"router" yaml:"router"`
	Scheduler     Service[scheduler.Options]     `json:"scheduler" yaml:"scheduler"`
	Search        Service[search.Options]        `json:"search" yaml:"search"`
	Secrets       Service[secrets.Options]       `json:"secrets" yaml:"secrets"`
	Session       Service[session.Options]       `json:"session" yaml:"session"`
	Storage       Service[storage.Options]       `json:"storage" yaml:"storage"`
	Tenant        Service[tenant.Options]        `json:"tenant" yaml:"tenant"`
	VectorStore   Service[vectorstore.Options]   `json:"vectorstore" yaml:"vectorstore"`
	Webhook       Service[webhook.Options]       `json:"webhook" yaml:"webhook"`
	Workflow      Service[workflow.Options]      `json:"workflow" yaml:"workflow"`
}

// knownServiceNames returns the 34 lowercase service names in sorted order.
// It is the single source of truth for env matching and error paths.
func knownServiceNames() []string {
	return []string{
		"ai", "analytics", "auth", "billing", "cache", "crypto", "db",
		"document", "eventbus", "flag", "geo", "i18n", "idempotency",
		"lock", "log", "mailer", "media", "notification", "observability",
		"password", "payment", "permission", "queue", "ratelimit",
		"router", "scheduler", "search", "secrets", "session", "storage",
		"tenant", "vectorstore", "webhook", "workflow",
	}
}

// mapCodec decodes freshly marshaled JSON into a plain map[string]any; see
// serviceToMap.
var mapCodec = codec.JSONCodec[map[string]any]{}

// serviceToMap converts one service's typed options to a plain map via a
// JSON round-trip, preserving each package's json-tag semantics. It is the
// shared bridge for redaction and env field discovery. The file path stays
// marshal-based too (decodeOptions), so reflection is confined to env.go.
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
	out := make(map[string]ServiceConfig, 34)
	put := func(name, adapter string, opts any) {
		out[name] = ServiceConfig{Adapter: adapter, Options: Redact(serviceToMap(opts))}
	}
	put("ai", c.AI.Adapter, c.AI.Options)
	put("analytics", c.Analytics.Adapter, c.Analytics.Options)
	put("auth", c.Auth.Adapter, c.Auth.Options)
	put("billing", c.Billing.Adapter, c.Billing.Options)
	put("cache", c.Cache.Adapter, c.Cache.Options)
	put("crypto", c.Crypto.Adapter, c.Crypto.Options)
	put("db", c.DB.Adapter, c.DB.Options)
	put("document", c.Document.Adapter, c.Document.Options)
	put("eventbus", c.Eventbus.Adapter, c.Eventbus.Options)
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
	put("password", c.Password.Adapter, c.Password.Options)
	put("payment", c.Payment.Adapter, c.Payment.Options)
	put("permission", c.Permission.Adapter, c.Permission.Options)
	put("queue", c.Queue.Adapter, c.Queue.Options)
	put("ratelimit", c.Ratelimit.Adapter, c.Ratelimit.Options)
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
	return out
}

// discoveryOrder lists the filenames probed (in CWD) when Load is called
// with an empty path. First hit wins; absent files are silently skipped.
var discoveryOrder = []string{"zever.yaml", "zever.yml", "zever.json"}

// Load builds a Config in layers, later layers winning:
//
//  1. Default: zero-infrastructure adapters plus deterministic dev secrets.
//  2. File: path != "" must exist and decode (any failure returns nil, err);
//     path == "" discovers zever.yaml/yml/json in CWD, absent files skip silently.
//  3. merge: strict — unknown services and unknown fields fail.
//  4. applyEnv: best-effort — unknown variables are ignored, never errors.
//  5. Validate: fail-closed — any service failure returns nil, err.
//
// On ANY failure Load returns nil plus the error.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		raw, err := decodeFile(path)
		if err != nil {
			return nil, err
		}
		if err := merge(cfg, raw); err != nil {
			return nil, err
		}
	} else if found := discover(); found != "" {
		raw, err := decodeFile(found)
		if err != nil {
			return nil, err
		}
		if err := merge(cfg, raw); err != nil {
			return nil, err
		}
	}
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// discover returns the first existing discoveryOrder file in CWD, or ""
// when none exists. Stat failures (missing files, permissions) simply
// skip: discovery is best-effort, and an explicit path stays strict.
func discover() string {
	for _, name := range discoveryOrder {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			return name
		}
	}
	return ""
}
