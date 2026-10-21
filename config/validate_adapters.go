package config

import "errors"

// serviceAdapter pairs a service name with its configured adapter string.
type serviceAdapter struct {
	service string
	adapter string
}

// adapters lists every service's configured adapter. It is the single table
// ValidateAdapters checks, so a new service needs exactly one line here (and
// in Validate).
func (c *Config) adapters() []serviceAdapter {
	return []serviceAdapter{
		{"ai", c.AI.Adapter},
		{"analytics", c.Analytics.Adapter},
		{"auth", c.Auth.Adapter},
		{"billing", c.Billing.Adapter},
		{"cache", c.Cache.Adapter},
		{"cdn", c.CDN.Adapter},
		{"crypto", c.Crypto.Adapter},
		{"db", c.DB.Adapter},
		{"document", c.Document.Adapter},
		{"eventbus", c.EventBus.Adapter},
		{"flag", c.Flag.Adapter},
		{"geo", c.Geo.Adapter},
		{"i18n", c.I18n.Adapter},
		{"idempotency", c.Idempotency.Adapter},
		{"lock", c.Lock.Adapter},
		{"log", c.Log.Adapter},
		{"mailer", c.Mailer.Adapter},
		{"media", c.Media.Adapter},
		{"notification", c.Notification.Adapter},
		{"observability", c.Observability.Adapter},
		{"outbox", c.Outbox.Adapter},
		{"password", c.Password.Adapter},
		{"payment", c.Payment.Adapter},
		{"permission", c.Permission.Adapter},
		{"queue", c.Queue.Adapter},
		{"ratelimit", c.RateLimit.Adapter},
		{"resilience", c.Resilience.Adapter},
		{"router", c.Router.Adapter},
		{"scheduler", c.Scheduler.Adapter},
		{"search", c.Search.Adapter},
		{"secrets", c.Secrets.Adapter},
		{"session", c.Session.Adapter},
		{"storage", c.Storage.Adapter},
		{"tenant", c.Tenant.Adapter},
		{"vectorstore", c.VectorStore.Adapter},
		{"webhook", c.Webhook.Adapter},
		{"workflow", c.Workflow.Adapter},
	}
}

// ValidateAdapters fails fast on configured adapters with no registered
// factory, joining all failures with errors.Join so one bad service never
// hides another. registered maps a service name to the adapter names with a
// factory in this process; the registry (not ParseAdapter, which stays
// permissive for custom adapters) is the source of truth, so callers pass
// container.RegisteredAdapters. An adapter missing from its service's list
// fails with UnknownAdapterError carrying a "did you mean %q?" hint when a
// registered name is close; an empty list means nothing was imported and
// every configured adapter fails (forgotten import). The failure carries the
// "config: " prefix of the error type itself, exactly once: the message
// reads "config: unknown adapter ...", never "config: config: ...".
func (c *Config) ValidateAdapters(registered func(service string) []string) error {
	return c.validateAdapters(nil, registered)
}

// EnabledServices returns the set of services the project actually uses: the
// explicitly configured names (top-level keys of the project config) plus
// every service whose adapter differs from Default(), which covers env
// overrides. Unconfigured services left at their default adapter are not
// enabled, so a slim binary need not register adapters it never uses.
func (c *Config) EnabledServices(explicit []string) map[string]bool {
	out := make(map[string]bool, len(explicit))
	for _, name := range explicit {
		out[name] = true
	}

	defaults := Default().adapters()
	for i, sa := range c.adapters() {
		if sa.adapter != defaults[i].adapter {
			out[sa.service] = true
		}
	}

	return out
}

// ValidateEnabledAdapters is ValidateAdapters scoped to the services in
// EnabledServices(explicit): unconfigured services at their default adapter
// are skipped, so a minimal scaffold registering only db and router passes.
// Enabled services with an unregistered adapter still fail with the
// did-you-mean hint, and custom adapters pass as long as they are registered.
func (c *Config) ValidateEnabledAdapters(explicit []string, registered func(service string) []string) error {
	return c.validateAdapters(c.EnabledServices(explicit), registered)
}

// validateAdapters checks every service when enabled is nil, else only the
// services in enabled.
func (c *Config) validateAdapters(enabled map[string]bool, registered func(service string) []string) error {
	var errs []error
	for _, sa := range c.adapters() {
		if enabled != nil && !enabled[sa.service] {
			continue
		}

		known := registered(sa.service)
		found := false
		for _, name := range known {
			if name == sa.adapter {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, &UnknownAdapterError{
				Service:    sa.service,
				Adapter:    sa.adapter,
				Suggestion: closest(sa.adapter, known),
			})
		}
	}
	return errors.Join(errs...)
}
