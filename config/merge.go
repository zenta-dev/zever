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
	"github.com/zenta-dev/zever/session"
	"github.com/zenta-dev/zever/storage"
	"github.com/zenta-dev/zever/tenant"
	"github.com/zenta-dev/zever/vectorstore"
	"github.com/zenta-dev/zever/webhook"
	"github.com/zenta-dev/zever/workflow"
)

// merge overlays raw file service envelopes onto cfg. It is strict: unknown
// service names fail with UnknownServiceError, and unknown option fields
// fail via decodeOptions (typed per service, values never echoed). An empty
// adapter keeps the default; empty/nil options keep the default options.
func merge(cfg *Config, raw map[string]ServiceConfig) error {
	for name, sc := range raw {
		apply, ok := serviceMergers[name]
		if !ok {
			return &UnknownServiceError{Service: name}
		}
		if err := apply(cfg, sc); err != nil {
			return err
		}
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
		return mergeInto[eventbus.Options]("eventbus", &cfg.Eventbus.Adapter, &cfg.Eventbus.Options, sc)
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
		return mergeInto[ratelimit.Options]("ratelimit", &cfg.Ratelimit.Adapter, &cfg.Ratelimit.Options, sc)
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
