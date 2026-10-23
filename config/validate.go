package config

import (
	"errors"
	"fmt"
)

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
// Backend selection is compile-time (container.Provide plus direct
// imports), so there are no adapter names to check: Options.Validate runs
// for every service. Plugins with a registered validator (see
// RegisterPluginValidator) run their hook too; unregistered plugins are
// opaque and skip validation.
func (c *Config) Validate() error {
	var errs []error
	check := func(svc string, validate func() error) {
		if validate != nil {
			if err := validate(); err != nil {
				errs = append(errs, fmt.Errorf("config: invalid %s options: %w", svc, err))
			}
		}
	}
	check("ai", c.AI.Options.Validate)
	check("analytics", c.Analytics.Options.Validate)
	check("auth", c.Auth.Options.Validate)
	check("billing", c.Billing.Options.Validate)
	check("cache", c.Cache.Options.Validate)
	check("cdn", c.CDN.Options.Validate)
	check("crypto", c.Crypto.Options.Validate)
	check("db", c.DB.Options.Validate)
	check("document", c.Document.Options.Validate)
	check("eventbus", c.EventBus.Options.Validate)
	check("flag", c.Flag.Options.Validate)
	check("geo", c.Geo.Options.Validate)
	check("i18n", c.I18n.Options.Validate)
	check("idempotency", c.Idempotency.Options.Validate)
	check("lock", c.Lock.Options.Validate)
	check("log", c.Log.Options.Validate)
	check("mailer", c.Mailer.Options.Validate)
	check("media", c.Media.Options.Validate)
	check("notification", c.Notification.Options.Validate)
	check("observability", c.Observability.Options.Validate)
	check("outbox", c.Outbox.Options.Validate)
	check("password", c.Password.Options.Validate)
	check("payment", c.Payment.Options.Validate)
	check("permission", c.Permission.Options.Validate)
	check("queue", c.Queue.Options.Validate)
	check("ratelimit", c.RateLimit.Options.Validate)
	check("resilience", c.Resilience.Options.Validate)
	check("router", c.Router.Options.Validate)
	check("scheduler", c.Scheduler.Options.Validate)
	check("search", c.Search.Options.Validate)
	check("secrets", c.Secrets.Options.Validate)
	check("session", c.Session.Options.Validate)
	check("storage", c.Storage.Options.Validate)
	check("tenant", c.Tenant.Options.Validate)
	check("vectorstore", c.VectorStore.Options.Validate)
	check("webhook", c.Webhook.Options.Validate)
	check("workflow", c.Workflow.Options.Validate)
	for _, name := range sortedPluginNames(c.Plugins) {
		svc := c.Plugins[name]
		validate, ok := lookupPluginValidator(name)
		if !ok {
			continue
		}
		if err := validate(svc.Options); err != nil {
			errs = append(errs, fmt.Errorf("config: invalid %s options: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
