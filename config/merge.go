package config

import (
	"encoding/json"

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

// merge overlays raw file service envelopes onto cfg. It is strict: unknown
// service names fail with UnknownServiceError, and unknown option fields
// fail via decodeOptions (typed per service, values never echoed). An empty
// adapter keeps the default; empty/nil options keep the default options.
// The top-level `plugins` key is exempt from the known-service check: it
// maps plugin names to {adapter, options} envelopes and fans out into
// cfg.Plugins via mergePlugins.
func merge(cfg *Config, raw map[string]ServiceConfig) error {
	for name, sc := range raw {
		if name == "plugins" {
			if err := mergePlugins(cfg, sc); err != nil {
				return err
			}
			continue
		}
		apply, ok := serviceMergers[name]
		if !ok {
			return &UnknownServiceError{Service: name, Suggestion: closest(name, knownServiceNames())}
		}
		if err := apply(cfg, sc); err != nil {
			return err
		}
	}
	return nil
}

// mergePlugins fans the top-level `plugins` block out into cfg.Plugins,
// initializing the map on first use. Each plugin entry must be a strict
// {adapter, options} envelope (unknown envelope keys fail, like core
// services), but the options inside pass through raw: unknown option
// fields never fail here because the plugin validates them itself (see
// RegisterPluginValidator). An empty adapter keeps the current value;
// empty/nil options keep the current raw options.
func mergePlugins(cfg *Config, sc ServiceConfig) error {
	if len(sc.Options) == 0 {
		return nil
	}
	if cfg.Plugins == nil {
		cfg.Plugins = make(map[string]Service[json.RawMessage], len(sc.Options))
	}
	for pname, pentry := range sc.Options {
		psc, err := decodeServiceEntry(pname, pentry)
		if err != nil {
			return err
		}
		cur := cfg.Plugins[pname]
		if psc.Adapter != "" {
			cur.Adapter = psc.Adapter
		}
		if len(psc.Options) > 0 {
			raw, err := json.Marshal(psc.Options)
			if err != nil {
				return &DecodeError{Service: pname, Err: err}
			}
			cur.Options = json.RawMessage(raw)
		}
		cfg.Plugins[pname] = cur
	}
	return nil
}

// mergeInto decodes sc.Options into dst via the generic strict decoder.
// Empty adapter keeps the current value; nil/empty options keep dst.
func mergeInto[T any](service string, adapter *string, dst *T, sc ServiceConfig) error {
	if sc.Adapter != "" {
		*adapter = sc.Adapter
	}
	if len(sc.Options) == 0 {
		return nil
	}
	o, err := decodeOptions[T](service, sc.Options)
	if err != nil {
		return err
	}
	*dst = o
	return nil
}

// serviceMergers maps each known service name to its typed merge
// function. A table (not twin switches) keeps strict dispatch in one
// place; knownServiceNames in config.go remains the sorted-name source.
var serviceMergers = map[string]func(*Config, ServiceConfig) error{
	"ai": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[ai.Options]("ai", &cfg.AI.Adapter, &cfg.AI.Options, sc)
	},
	"analytics": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[analytics.Options]("analytics", &cfg.Analytics.Adapter, &cfg.Analytics.Options, sc)
	},
	"auth": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[auth.Options]("auth", &cfg.Auth.Adapter, &cfg.Auth.Options, sc)
	},
	"billing": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[billing.Options]("billing", &cfg.Billing.Adapter, &cfg.Billing.Options, sc)
	},
	"cache": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[cache.Options]("cache", &cfg.Cache.Adapter, &cfg.Cache.Options, sc)
	},
	"crypto": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[crypto.Options]("crypto", &cfg.Crypto.Adapter, &cfg.Crypto.Options, sc)
	},
	"db": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[db.Options]("db", &cfg.DB.Adapter, &cfg.DB.Options, sc)
	},
	"document": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[document.Options]("document", &cfg.Document.Adapter, &cfg.Document.Options, sc)
	},
	"eventbus": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[eventbus.Options]("eventbus", &cfg.EventBus.Adapter, &cfg.EventBus.Options, sc)
	},
	"flag": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[flag.Options]("flag", &cfg.Flag.Adapter, &cfg.Flag.Options, sc)
	},
	"geo": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[geo.Options]("geo", &cfg.Geo.Adapter, &cfg.Geo.Options, sc)
	},
	"i18n": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[i18n.Options]("i18n", &cfg.I18n.Adapter, &cfg.I18n.Options, sc)
	},
	"idempotency": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[idempotency.Options]("idempotency", &cfg.Idempotency.Adapter, &cfg.Idempotency.Options, sc)
	},
	"lock": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[lock.Options]("lock", &cfg.Lock.Adapter, &cfg.Lock.Options, sc)
	},
	"log": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[log.Options]("log", &cfg.Log.Adapter, &cfg.Log.Options, sc)
	},
	"mailer": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[mailer.Options]("mailer", &cfg.Mailer.Adapter, &cfg.Mailer.Options, sc)
	},
	"media": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[media.Options]("media", &cfg.Media.Adapter, &cfg.Media.Options, sc)
	},
	"notification": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[notification.Options]("notification", &cfg.Notification.Adapter, &cfg.Notification.Options, sc)
	},
	"observability": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[observability.Options]("observability", &cfg.Observability.Adapter, &cfg.Observability.Options, sc)
	},
	"password": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[password.Options]("password", &cfg.Password.Adapter, &cfg.Password.Options, sc)
	},
	"payment": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[payment.Options]("payment", &cfg.Payment.Adapter, &cfg.Payment.Options, sc)
	},
	"permission": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[permission.Options]("permission", &cfg.Permission.Adapter, &cfg.Permission.Options, sc)
	},
	"queue": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[queue.Options]("queue", &cfg.Queue.Adapter, &cfg.Queue.Options, sc)
	},
	"ratelimit": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[ratelimit.Options]("ratelimit", &cfg.RateLimit.Adapter, &cfg.RateLimit.Options, sc)
	},
	"router": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[router.Options]("router", &cfg.Router.Adapter, &cfg.Router.Options, sc)
	},
	"scheduler": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[scheduler.Options]("scheduler", &cfg.Scheduler.Adapter, &cfg.Scheduler.Options, sc)
	},
	"search": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[search.Options]("search", &cfg.Search.Adapter, &cfg.Search.Options, sc)
	},
	"secrets": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[secrets.Options]("secrets", &cfg.Secrets.Adapter, &cfg.Secrets.Options, sc)
	},
	"session": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[session.Options]("session", &cfg.Session.Adapter, &cfg.Session.Options, sc)
	},
	"storage": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[storage.Options]("storage", &cfg.Storage.Adapter, &cfg.Storage.Options, sc)
	},
	"tenant": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[tenant.Options]("tenant", &cfg.Tenant.Adapter, &cfg.Tenant.Options, sc)
	},
	"vectorstore": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[vectorstore.Options]("vectorstore", &cfg.VectorStore.Adapter, &cfg.VectorStore.Options, sc)
	},
	"webhook": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[webhook.Options]("webhook", &cfg.Webhook.Adapter, &cfg.Webhook.Options, sc)
	},
	"workflow": func(cfg *Config, sc ServiceConfig) error {
		return mergeInto[workflow.Options]("workflow", &cfg.Workflow.Adapter, &cfg.Workflow.Options, sc)
	},
}
