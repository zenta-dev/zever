package container

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/core/agent"
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
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/rag"
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
	"github.com/zenta-dev/zever/shared/grpcclient"
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

// Agent builds a tool-calling *agent.Loop over the resolved AI backend. It has
// no adapter registry and no config entry; the model name comes from the AI
// options. Like Job, it shares the process-wide backend instance.
func (c *Container) Agent() (*agent.Loop, error) {
	return c.agent.get(func() (*agent.Loop, error) {
		a, err := c.AI()
		if err != nil {
			return nil, fmt.Errorf("container: agent: resolve ai: %w", err)
		}

		loop, err := agent.New(a, agent.Options{Model: c.cfg.AI.Options.Model})
		if err != nil {
			return nil, fmt.Errorf("container: agent: %w", err)
		}

		return loop, nil
	})
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

// Cache resolves and returns the cache service instance. A db-backed cache
// pointing at an already-pooled DSN borrows the shared pool (owns=false);
// DedicatedPool or a non-shareable DSN opens a private pool as before.
func (c *Container) Cache() (cache.Cache, error) {
	return c.cache.get(func() (cache.Cache, error) {
		if v, shared, err := c.openSharedCache(); shared || err != nil {
			return v, err
		}

		return openService("cache", c.cfg.Cache.Adapter, cache.ParseAdapter, cache.Open, c.cfg.Cache.Options)
	})
}

// Crypto resolves and returns the crypto service instance.
func (c *Container) Crypto() (crypto.Crypto, error) {
	return c.crypto.get(func() (crypto.Crypto, error) {
		return openService("crypto", c.cfg.Crypto.Adapter, crypto.ParseAdapter, crypto.Open, c.cfg.Crypto.Options)
	})
}

// DB resolves and returns the db service instance. Shareable pools register
// in the DSN registry for borrowers; DedicatedPool is meaningless on db
// itself and is ignored.
func (c *Container) DB() (db.DB, error) {
	return c.db.get(func() (db.DB, error) {
		return c.openDB()
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

// grpcClientCache holds one lazily built *grpc.ClientConn per target
// string (see GRPCClient). The mutex makes the cache goroutine-safe; the
// lazy wrapper gives singleflight build semantics for the cache itself.
type grpcClientCache struct {
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

// get returns the cached conn for target, building and caching one on
// first use. A failed build is not cached: the error returns to the caller
// and the next call retries.
func (c *grpcClientCache) get(target string, build func() (*grpc.ClientConn, error)) (*grpc.ClientConn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, ok := c.conns[target]; ok {
		return conn, nil
	}

	conn, err := build()
	if err != nil {
		return nil, err
	}

	c.conns[target] = conn

	return conn, nil
}

// GRPCClient resolves and returns a *grpc.ClientConn for target, built by
// shared/grpcclient and cached per target string: repeat calls with the
// same target return the same conn, while distinct targets get distinct
// conns. Options apply to the build only; later calls with different
// options keep the first build. The conns close with the container.
// grpcclient.New performs no IO, so resolving never dials.
func (c *Container) GRPCClient(target string, opts ...grpcclient.Option) (*grpc.ClientConn, error) {
	cache, err := c.grpcClients.get(func() (grpcClientCache, error) {
		return grpcClientCache{conns: make(map[string]*grpc.ClientConn)}, nil
	})
	if err != nil {
		return nil, err
	}

	return cache.get(target, func() (*grpc.ClientConn, error) {
		return grpcclient.New(context.Background(), target, opts...)
	})
}

// I18n resolves and returns the i18n service instance.
func (c *Container) I18n() (i18n.I18n, error) {
	return c.i18n.get(func() (i18n.I18n, error) {
		return openService("i18n", c.cfg.I18n.Adapter, i18n.ParseAdapter, i18n.Open, c.cfg.I18n.Options)
	})
}

// Idempotency resolves and returns the idempotency service instance. A
// db-backed store pointing at an already-pooled DSN borrows the shared
// pool (owns=false); DedicatedPool or a non-shareable DSN opens a private
// pool as before.
func (c *Container) Idempotency() (idempotency.Store, error) {
	return c.idempotency.get(func() (idempotency.Store, error) {
		if v, shared, err := c.openSharedIdempotency(); shared || err != nil {
			return v, err
		}

		return openService("idempotency", c.cfg.Idempotency.Adapter, idempotency.ParseAdapter, idempotency.Open, c.cfg.Idempotency.Options)
	})
}

// Job builds a job.Dispatcher over the resolved queue service. Job has no
// adapter registry of its own: it shares the process-wide queue instance
// instead of opening redundant connections. Logger stays zero, which the
// dispatcher treats as a noop default.
//
// Trace propagation: Dispatch carries the caller's W3C trace context into the
// message headers (queue adapters inject on Push/PushDelayed via
// shared/traceprop), so workers that pop from the shared queue can extract it
// with traceprop.Extract and continue the trace with a consumer child span
// (traceprop.StartConsumeSpan). The dispatcher wraps the shared queue
// unwrapped: d.Q is the process-wide Queue instance.
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

// Outbox resolves and returns the outbox service instance. The db adapter
// opens its own pool from Options.DSN (empty selects a private in-memory
// database); container-level pool sharing is not wired for outbox.
func (c *Container) Outbox() (outbox.Store, error) {
	return c.outbox.get(func() (outbox.Store, error) {
		opts := c.cfg.Outbox.Options
		// Best-effort: inject the resolved observability provider so the relay
		// emits metrics/spans. A failed or unconfigured observability service
		// never blocks the outbox.
		if obs, err := c.Observability(); err == nil {
			opts.Provider = obs
		}

		return openService("outbox", c.cfg.Outbox.Adapter, outbox.ParseAdapter, outbox.Open, opts)
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

// Queue resolves and returns the queue service instance. A db-backed queue
// pointing at an already-pooled DSN borrows the shared pool (owns=false);
// DedicatedPool or a non-shareable DSN opens a private pool as before.
// Under sustained load prefer a separate queue database with
// DedicatedPool (Solid Queue guidance).
func (c *Container) Queue() (queue.Queue, error) {
	return c.queue.get(func() (queue.Queue, error) {
		if v, shared, err := c.openSharedQueue(); shared || err != nil {
			return v, err
		}

		return openService("queue", c.cfg.Queue.Adapter, queue.ParseAdapter, queue.Open, c.cfg.Queue.Options)
	})
}

// RAG builds a *rag.Engine over the resolved AI and VectorStore backends. It
// has no adapter registry and no config entry. Like Job, it shares the
// process-wide backend instances.
func (c *Container) RAG() (*rag.Engine, error) {
	return c.rag.get(func() (*rag.Engine, error) {
		a, err := c.AI()
		if err != nil {
			return nil, fmt.Errorf("container: rag: resolve ai: %w", err)
		}

		vs, err := c.VectorStore()
		if err != nil {
			return nil, fmt.Errorf("container: rag: resolve vectorstore: %w", err)
		}

		engine, err := rag.New(a, vs, rag.Options{Model: c.cfg.AI.Options.Model})
		if err != nil {
			return nil, fmt.Errorf("container: rag: %w", err)
		}

		return engine, nil
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

// Resilience resolves and returns the resilience service instance.
func (c *Container) Resilience() (resilience.Manager, error) {
	return c.resilience.get(func() (resilience.Manager, error) {
		return openService("resilience", c.cfg.Resilience.Adapter, resilience.ParseAdapter, resilience.Open, c.cfg.Resilience.Options)
	})
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
// Dispatcher is used as-is. A postgres scheduler pointing at an
// already-pooled DSN borrows the shared pool (owns=false); DedicatedPool
// or a non-shareable DSN opens a private pool as before.
func (c *Container) Scheduler() (scheduler.Scheduler, error) {
	return c.scheduler.get(func() (scheduler.Scheduler, error) {
		if v, shared, err := c.openSharedScheduler(); shared || err != nil {
			return v, err
		}

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

// Search resolves and returns the search service instance. A db-backed
// search (canonical "db" or its "postgres"/"sqlite" aliases) pointing at
// an already-pooled DSN borrows the shared pool (owns=false);
// DedicatedPool or a non-shareable DSN opens a private pool as before.
func (c *Container) Search() (search.Search, error) {
	return c.search.get(func() (search.Search, error) {
		if v, shared, err := c.openSharedSearch(); shared || err != nil {
			return v, err
		}

		return openService("search", c.cfg.Search.Adapter, search.ParseAdapter, search.Open, c.cfg.Search.Options)
	})
}

// Secrets resolves and returns the secrets service instance.
func (c *Container) Secrets() (secrets.Secrets, error) {
	return c.secrets.get(func() (secrets.Secrets, error) {
		return openService("secrets", c.cfg.Secrets.Adapter, secrets.ParseAdapter, secrets.Open, c.cfg.Secrets.Options)
	})
}

// Session resolves and returns the session service instance. A db-backed
// session pointing at an already-pooled DSN borrows the shared pool
// (owns=false); DedicatedPool or a non-shareable DSN opens a private pool
// as before.
func (c *Container) Session() (session.Store, error) {
	return c.session.get(func() (session.Store, error) {
		if v, shared, err := c.openSharedSession(); shared || err != nil {
			return v, err
		}

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

// VectorStore resolves and returns the vectorstore service instance. A
// db-backed vectorstore (canonical "db" or its "pgvector"/"sqlite"
// aliases) pointing at an already-pooled DSN borrows the shared pool
// (owns=false); DedicatedPool or a non-shareable DSN opens a private pool
// as before.
func (c *Container) VectorStore() (vectorstore.VectorStore, error) {
	return c.vectorstore.get(func() (vectorstore.VectorStore, error) {
		if v, shared, err := c.openSharedVectorStore(); shared || err != nil {
			return v, err
		}

		return openService("vectorstore", c.cfg.VectorStore.Adapter, vectorstore.ParseAdapter, vectorstore.Open, c.cfg.VectorStore.Options)
	})
}

// Webhook resolves and returns the webhook service instance.
func (c *Container) Webhook() (webhook.Webhook, error) {
	return c.webhook.get(func() (webhook.Webhook, error) {
		return openService("webhook", c.cfg.Webhook.Adapter, webhook.ParseAdapter, webhook.Open, c.cfg.Webhook.Options)
	})
}

// Workflow resolves and returns the workflow service instance. A db-backed
// workflow (canonical "db" or its "postgres" alias) pointing at an
// already-pooled DSN borrows the shared pool (owns=false); DedicatedPool
// or a non-shareable DSN opens a private pool as before. Under sustained
// load prefer a separate workflow database with DedicatedPool.
func (c *Container) Workflow() (workflow.Workflow, error) {
	return c.workflow.get(func() (workflow.Workflow, error) {
		if v, shared, err := c.openSharedWorkflow(); shared || err != nil {
			return v, err
		}

		return openService("workflow", c.cfg.Workflow.Adapter, workflow.ParseAdapter, workflow.Open, c.cfg.Workflow.Options)
	})
}
