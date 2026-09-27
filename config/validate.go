package config

import (
	"errors"
	"fmt"

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

// parseAsAny adapts a package's typed ParseAdapter to the untyped check
// hook below.
func parseAsAny[T any](parse func(string) (T, error)) func(string) (any, error) {
	return func(s string) (any, error) { return parse(s) }
}

// Validate checks every service fail-closed and joins all failures with
// errors.Join, so one bad service never hides another. Each failure is
// wrapped once as `config: invalid <service> options: %w`.
//
// Wrapping choice: per-service Validate errors are already typed
// (InvalidOptionsError, ErrInvalidHash, ...). Re-wrapping them in this
// package's InvalidOptionsError{Reason: err.Error()} would stringify and
// duplicate the reason while destroying the chain. fmt.Errorf with %w
// preserves errors.Is/As into the original typed errors while still
// naming the service for DX.
//
// Adapter names go through each package's ParseAdapter (all 34 services
// provide one). Options.Validate runs everywhere it exists. Plugins with a
// registered validator (see RegisterPluginValidator) run their hook too;
// unregistered plugins are opaque and skip validation.
func (c *Config) Validate() error {
	var errs []error
	check := func(svc, adapter string, parse func(string) (any, error), validate func() error) {
		if _, err := parse(adapter); err != nil {
			errs = append(errs, fmt.Errorf("config: invalid %s options: %w", svc, err))
		}
		if validate != nil {
			if err := validate(); err != nil {
				errs = append(errs, fmt.Errorf("config: invalid %s options: %w", svc, err))
			}
		}
	}
	check("ai", c.AI.Adapter, parseAsAny(ai.ParseAdapter), c.AI.Options.Validate)
	check("analytics", c.Analytics.Adapter, parseAsAny(analytics.ParseAdapter), c.Analytics.Options.Validate)
	check("auth", c.Auth.Adapter, parseAsAny(auth.ParseAdapter), c.Auth.Options.Validate)
	check("billing", c.Billing.Adapter, parseAsAny(billing.ParseAdapter), c.Billing.Options.Validate)
	check("cache", c.Cache.Adapter, parseAsAny(cache.ParseAdapter), c.Cache.Options.Validate)
	check("crypto", c.Crypto.Adapter, parseAsAny(crypto.ParseAdapter), c.Crypto.Options.Validate)
	check("db", c.DB.Adapter, parseAsAny(db.ParseAdapter), c.DB.Options.Validate)
	check("document", c.Document.Adapter, parseAsAny(document.ParseAdapter), c.Document.Options.Validate)
	check("eventbus", c.EventBus.Adapter, parseAsAny(eventbus.ParseAdapter), c.EventBus.Options.Validate)
	check("flag", c.Flag.Adapter, parseAsAny(flag.ParseAdapter), c.Flag.Options.Validate)
	check("geo", c.Geo.Adapter, parseAsAny(geo.ParseAdapter), c.Geo.Options.Validate)
	check("i18n", c.I18n.Adapter, parseAsAny(i18n.ParseAdapter), c.I18n.Options.Validate)
	check("idempotency", c.Idempotency.Adapter, parseAsAny(idempotency.ParseAdapter), c.Idempotency.Options.Validate)
	check("lock", c.Lock.Adapter, parseAsAny(lock.ParseAdapter), c.Lock.Options.Validate)
	check("log", c.Log.Adapter, parseAsAny(log.ParseAdapter), c.Log.Options.Validate)
	check("mailer", c.Mailer.Adapter, parseAsAny(mailer.ParseAdapter), c.Mailer.Options.Validate)
	check("media", c.Media.Adapter, parseAsAny(media.ParseAdapter), c.Media.Options.Validate)
	check("notification", c.Notification.Adapter, parseAsAny(notification.ParseAdapter), c.Notification.Options.Validate)
	check("observability", c.Observability.Adapter, parseAsAny(observability.ParseAdapter), c.Observability.Options.Validate)
	check("password", c.Password.Adapter, parseAsAny(password.ParseAdapter), c.Password.Options.Validate)
	check("payment", c.Payment.Adapter, parseAsAny(payment.ParseAdapter), c.Payment.Options.Validate)
	check("permission", c.Permission.Adapter, parseAsAny(permission.ParseAdapter), c.Permission.Options.Validate)
	check("queue", c.Queue.Adapter, parseAsAny(queue.ParseAdapter), c.Queue.Options.Validate)
	check("ratelimit", c.RateLimit.Adapter, parseAsAny(ratelimit.ParseAdapter), c.RateLimit.Options.Validate)
	check("router", c.Router.Adapter, parseAsAny(router.ParseAdapter), c.Router.Options.Validate)
	check("scheduler", c.Scheduler.Adapter, parseAsAny(scheduler.ParseAdapter), c.Scheduler.Options.Validate)
	check("search", c.Search.Adapter, parseAsAny(search.ParseAdapter), c.Search.Options.Validate)
	check("secrets", c.Secrets.Adapter, parseAsAny(secrets.ParseAdapter), c.Secrets.Options.Validate)
	check("session", c.Session.Adapter, parseAsAny(session.ParseAdapter), c.Session.Options.Validate)
	check("storage", c.Storage.Adapter, parseAsAny(storage.ParseAdapter), c.Storage.Options.Validate)
	check("tenant", c.Tenant.Adapter, parseAsAny(tenant.ParseAdapter), c.Tenant.Options.Validate)
	check("vectorstore", c.VectorStore.Adapter, parseAsAny(vectorstore.ParseAdapter), c.VectorStore.Options.Validate)
	check("webhook", c.Webhook.Adapter, parseAsAny(webhook.ParseAdapter), c.Webhook.Options.Validate)
	check("workflow", c.Workflow.Adapter, parseAsAny(workflow.ParseAdapter), c.Workflow.Options.Validate)
	for _, name := range sortedPluginNames(c.Plugins) {
		svc := c.Plugins[name]
		validate, ok := lookupPluginValidator(name)
		if !ok {
			continue
		}
		if err := validate(svc.Adapter, svc.Options); err != nil {
			errs = append(errs, fmt.Errorf("config: invalid %s options: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
