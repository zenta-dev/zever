package config

import (
	"errors"
	"fmt"

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
// provide one). Options.Validate runs everywhere it exists.
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
	return errors.Join(errs...)
}
