package container

import (
	"fmt"

	"google.golang.org/grpc"

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
	"github.com/zenta-dev/zever/core/job"
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

// openService parses a service's adapter name, then opens it with typed
// opts. Adapters register caller-side via each battery's Register; resolving
// an unregistered adapter fails with UnknownAdapterError. The adapter string
// before any construction. Wrapping adds only the service name, so
// secret-bearing option values never leak into error text.
func openService[T any, A any, O any](service string, name string, parse func(string) (A, error), open func(A, O) (T, error), opts O) (T, error) {
	a, err := parse(name)
	if err != nil {
		var zero T

		return zero, fmt.Errorf("container: %s: %w", service, err)
	}

	v, err := open(a, opts)
	if err != nil {
		var zero T

		return zero, fmt.Errorf("container: %s: %w", service, err)
	}

	return v, nil
}

// AI resolves and returns the AI service instance.
func (c *Container) AI() (ai.AI, error) {
	return c.ai.get(func() (ai.AI, error) {
		return openService("ai", c.cfg.AI.Adapter, ai.ParseAdapter, ai.Open, c.cfg.AI.Options)
	})
}

// Analytics resolves and returns the analytics service instance.
func (c *Container) Analytics() (analytics.Analytics, error) {
	return c.analytics.get(func() (analytics.Analytics, error) {
		return openService("analytics", c.cfg.Analytics.Adapter, analytics.ParseAdapter, analytics.Open, c.cfg.Analytics.Options)
	})
}

// Auth resolves and returns the auth service instance.
func (c *Container) Auth() (auth.Auth, error) {
	return c.auth.get(func() (auth.Auth, error) {
		return openService("auth", c.cfg.Auth.Adapter, auth.ParseAdapter, auth.Open, c.cfg.Auth.Options)
	})
}

// Billing resolves and returns the billing service instance.
func (c *Container) Billing() (billing.Billing, error) {
	return c.billing.get(func() (billing.Billing, error) {
		return openService("billing", c.cfg.Billing.Adapter, billing.ParseAdapter, billing.Open, c.cfg.Billing.Options)
	})
}

// Cache resolves and returns the cache service instance.
func (c *Container) Cache() (cache.Cache, error) {
	return c.cache.get(func() (cache.Cache, error) {
		return openService("cache", c.cfg.Cache.Adapter, cache.ParseAdapter, cache.Open, c.cfg.Cache.Options)
	})
}

// Crypto resolves and returns the crypto service instance.
func (c *Container) Crypto() (crypto.Crypto, error) {
	return c.crypto.get(func() (crypto.Crypto, error) {
		return openService("crypto", c.cfg.Crypto.Adapter, crypto.ParseAdapter, crypto.Open, c.cfg.Crypto.Options)
	})
}

// DB resolves and returns the db service instance.
func (c *Container) DB() (db.DB, error) {
	return c.db.get(func() (db.DB, error) {
		return openService("db", c.cfg.DB.Adapter, db.ParseAdapter, db.Open, c.cfg.DB.Options)
	})
}

// Transactor resolves the db service and asserts it implements
// db.Transactor. Adapters without transaction support fail with a
// TransactorError naming the concrete type.
func (c *Container) Transactor() (db.Transactor, error) {
	d, err := c.DB()
	if err != nil {
		return nil, err
	}

	tr, ok := d.(db.Transactor)
	if !ok {
		return nil, TransactorError{Actual: fmt.Sprintf("%T", d)}
	}

	return tr, nil
}

// Document resolves and returns the document service instance.
func (c *Container) Document() (document.Document, error) {
	return c.document.get(func() (document.Document, error) {
		return openService("document", c.cfg.Document.Adapter, document.ParseAdapter, document.Open, c.cfg.Document.Options)
	})
}

// EventBus resolves and returns the eventbus service instance.
func (c *Container) EventBus() (eventbus.EventBus, error) {
	return c.eventbus.get(func() (eventbus.EventBus, error) {
		return openService("eventbus", c.cfg.EventBus.Adapter, eventbus.ParseAdapter, eventbus.Open, c.cfg.EventBus.Options)
	})
}

// Eventbus aliases EventBus for compatibility.
func (c *Container) Eventbus() (eventbus.EventBus, error) {
	return c.EventBus()
}

// Flag resolves and returns the flag service instance.
func (c *Container) Flag() (flag.Flag, error) {
	return c.flag.get(func() (flag.Flag, error) {
		return openService("flag", c.cfg.Flag.Adapter, flag.ParseAdapter, flag.Open, c.cfg.Flag.Options)
	})
}

// Geo resolves and returns the geo service instance.
func (c *Container) Geo() (geo.Geo, error) {
	return c.geo.get(func() (geo.Geo, error) {
		return openService("geo", c.cfg.Geo.Adapter, geo.ParseAdapter, geo.Open, c.cfg.Geo.Options)
	})
}

// GRPC resolves and returns the process-wide *grpc.Server as a lazy
// singleton: exactly one instance, built on first call and shared after.
// Options passed on later calls are ignored, since a server's interceptor
// chain is fixed at construction and cannot change once built.
func (c *Container) GRPC(opts ...grpc.ServerOption) (*grpc.Server, error) {
	return c.grpcServer.get(func() (*grpc.Server, error) {
		return grpc.NewServer(opts...), nil
	})
}

// I18n resolves and returns the i18n service instance.
func (c *Container) I18n() (i18n.I18n, error) {
	return c.i18n.get(func() (i18n.I18n, error) {
		return openService("i18n", c.cfg.I18n.Adapter, i18n.ParseAdapter, i18n.Open, c.cfg.I18n.Options)
	})
}

// Idempotency resolves and returns the idempotency service instance.
func (c *Container) Idempotency() (idempotency.Store, error) {
	return c.idempotency.get(func() (idempotency.Store, error) {
		return openService("idempotency", c.cfg.Idempotency.Adapter, idempotency.ParseAdapter, idempotency.Open, c.cfg.Idempotency.Options)
	})
}

// Job builds a job.Dispatcher over the resolved queue service. Job has no
// adapter registry of its own: it shares the process-wide queue instance
// instead of opening redundant connections. Logger stays zero, which the
// dispatcher treats as a noop default.
func (c *Container) Job() (*job.Dispatcher, error) {
	return c.job.get(func() (*job.Dispatcher, error) {
		q, err := c.Queue()
		if err != nil {
			return nil, fmt.Errorf("container: job: resolve queue: %w", err)
		}

		return &job.Dispatcher{Q: q}, nil
	})
}

// Lock resolves and returns the lock service instance.
func (c *Container) Lock() (lock.Locker, error) {
	return c.lock.get(func() (lock.Locker, error) {
		return openService("lock", c.cfg.Lock.Adapter, lock.ParseAdapter, lock.Open, c.cfg.Lock.Options)
	})
}

// Log resolves and returns the log service instance.
func (c *Container) Log() (log.Logger, error) {
	return c.log.get(func() (log.Logger, error) {
		return openService("log", c.cfg.Log.Adapter, log.ParseAdapter, log.Open, c.cfg.Log.Options)
	})
}

// Mailer resolves and returns the mailer service instance.
func (c *Container) Mailer() (mailer.Mailer, error) {
	return c.mailer.get(func() (mailer.Mailer, error) {
		return openService("mailer", c.cfg.Mailer.Adapter, mailer.ParseAdapter, mailer.Open, c.cfg.Mailer.Options)
	})
}

// Media resolves and returns the media service instance.
func (c *Container) Media() (media.Media, error) {
	return c.media.get(func() (media.Media, error) {
		return openService("media", c.cfg.Media.Adapter, media.ParseAdapter, media.Open, c.cfg.Media.Options)
	})
}

// Notification resolves and returns the notification service instance.
func (c *Container) Notification() (notification.Notifier, error) {
	return c.notification.get(func() (notification.Notifier, error) {
		return openService("notification", c.cfg.Notification.Adapter, notification.ParseAdapter, notification.Open, c.cfg.Notification.Options)
	})
}

// Observability resolves and returns the observability service instance.
func (c *Container) Observability() (observability.Provider, error) {
	return c.observability.get(func() (observability.Provider, error) {
		return openService("observability", c.cfg.Observability.Adapter, observability.ParseAdapter, observability.Open, c.cfg.Observability.Options)
	})
}

// Password resolves and returns the password service instance.
func (c *Container) Password() (password.Hasher, error) {
	return c.password.get(func() (password.Hasher, error) {
		return openService("password", c.cfg.Password.Adapter, password.ParseAdapter, password.Open, c.cfg.Password.Options)
	})
}

// Payment resolves and returns the payment service instance.
func (c *Container) Payment() (payment.Payment, error) {
	return c.payment.get(func() (payment.Payment, error) {
		return openService("payment", c.cfg.Payment.Adapter, payment.ParseAdapter, payment.Open, c.cfg.Payment.Options)
	})
}

// Permission resolves and returns the permission service instance.
func (c *Container) Permission() (permission.Checker, error) {
	return c.permission.get(func() (permission.Checker, error) {
		return openService("permission", c.cfg.Permission.Adapter, permission.ParseAdapter, permission.Open, c.cfg.Permission.Options)
	})
}

// Queue resolves and returns the queue service instance.
func (c *Container) Queue() (queue.Queue, error) {
	return c.queue.get(func() (queue.Queue, error) {
		return openService("queue", c.cfg.Queue.Adapter, queue.ParseAdapter, queue.Open, c.cfg.Queue.Options)
	})
}

// RateLimit resolves and returns the ratelimit service instance.
func (c *Container) RateLimit() (ratelimit.Limiter, error) {
	return c.ratelimit.get(func() (ratelimit.Limiter, error) {
		return openService("ratelimit", c.cfg.RateLimit.Adapter, ratelimit.ParseAdapter, ratelimit.Open, c.cfg.RateLimit.Options)
	})
}

// Ratelimit aliases RateLimit for compatibility.
func (c *Container) Ratelimit() (ratelimit.Limiter, error) {
	return c.RateLimit()
}

// Router resolves and returns the router service instance.
func (c *Container) Router() (router.Router, error) {
	return c.router.get(func() (router.Router, error) {
		return openService("router", c.cfg.Router.Adapter, router.ParseAdapter, router.Open, c.cfg.Router.Options)
	})
}

// Scheduler resolves the scheduler service. A nil Dispatcher in options is
// replaced with the resolved job dispatcher, so ticks share the
// process-wide queue instead of opening redundant connections. An explicit
// Dispatcher is used as-is.
func (c *Container) Scheduler() (scheduler.Scheduler, error) {
	return c.scheduler.get(func() (scheduler.Scheduler, error) {
		opts := c.cfg.Scheduler.Options
		if opts.Dispatcher == nil {
			d, err := c.Job()
			if err != nil {
				return nil, fmt.Errorf("container: scheduler: resolve job: %w", err)
			}

			opts.Dispatcher = d
		}

		return openService("scheduler", c.cfg.Scheduler.Adapter, scheduler.ParseAdapter, scheduler.Open, opts)
	})
}

// Search resolves and returns the search service instance.
func (c *Container) Search() (search.Search, error) {
	return c.search.get(func() (search.Search, error) {
		return openService("search", c.cfg.Search.Adapter, search.ParseAdapter, search.Open, c.cfg.Search.Options)
	})
}

// Secrets resolves and returns the secrets service instance.
func (c *Container) Secrets() (secrets.Secrets, error) {
	return c.secrets.get(func() (secrets.Secrets, error) {
		return openService("secrets", c.cfg.Secrets.Adapter, secrets.ParseAdapter, secrets.Open, c.cfg.Secrets.Options)
	})
}

// Session resolves and returns the session service instance.
func (c *Container) Session() (session.Store, error) {
	return c.session.get(func() (session.Store, error) {
		return openService("session", c.cfg.Session.Adapter, session.ParseAdapter, session.Open, c.cfg.Session.Options)
	})
}

// Storage resolves and returns the storage service instance.
func (c *Container) Storage() (storage.Storage, error) {
	return c.storage.get(func() (storage.Storage, error) {
		return openService("storage", c.cfg.Storage.Adapter, storage.ParseAdapter, storage.Open, c.cfg.Storage.Options)
	})
}

// Tenant resolves and returns the tenant service instance.
func (c *Container) Tenant() (tenant.Tenant, error) {
	return c.tenant.get(func() (tenant.Tenant, error) {
		return openService("tenant", c.cfg.Tenant.Adapter, tenant.ParseAdapter, tenant.Open, c.cfg.Tenant.Options)
	})
}

// VectorStore resolves and returns the vectorstore service instance.
func (c *Container) VectorStore() (vectorstore.VectorStore, error) {
	return c.vectorstore.get(func() (vectorstore.VectorStore, error) {
		return openService("vectorstore", c.cfg.VectorStore.Adapter, vectorstore.ParseAdapter, vectorstore.Open, c.cfg.VectorStore.Options)
	})
}

// Webhook resolves and returns the webhook service instance.
func (c *Container) Webhook() (webhook.Webhook, error) {
	return c.webhook.get(func() (webhook.Webhook, error) {
		return openService("webhook", c.cfg.Webhook.Adapter, webhook.ParseAdapter, webhook.Open, c.cfg.Webhook.Options)
	})
}

// Workflow resolves and returns the workflow service instance.
func (c *Container) Workflow() (workflow.Workflow, error) {
	return c.workflow.get(func() (workflow.Workflow, error) {
		return openService("workflow", c.cfg.Workflow.Adapter, workflow.ParseAdapter, workflow.Open, c.cfg.Workflow.Options)
	})
}
