package container

import (
	"fmt"
	"sync"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/core/agent"
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

// Agent builds a tool-calling *agent.Loop over the resolved AI backend. It has
// no adapter registry and no config entry; the model name comes from the AI
// options. Like Job, it shares the process-wide backend instance. Options
// apply only on first resolution; later calls return the cached singleton.
func (c *Container) Agent(opts ...AgentOption) (*agent.Loop, error) {
	return c.agent.get(func() (*agent.Loop, error) {
		a, err := c.AI()
		if err != nil {
			return nil, fmt.Errorf("container: agent: resolve ai: %w", err)
		}

		aopts := agent.Options{Model: c.cfg.AI.Options.Model}
		for _, opt := range opts {
			if opt == nil {
				continue
			}

			opt(&aopts)
		}

		loop, err := agent.New(a, aopts)
		if err != nil {
			return nil, fmt.Errorf("container: agent: %w", err)
		}

		return loop, nil
	})
}

// AI resolves the wired ai service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) AI() (ai.AI, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(ai.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, ai.DefaultRef())
}

// Analytics resolves the wired analytics service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Analytics() (analytics.Analytics, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(analytics.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, analytics.DefaultRef())
}

// Auth resolves the wired auth service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Auth() (auth.Auth, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(auth.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, auth.DefaultRef())
}

// Billing resolves the wired billing service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Billing() (billing.Billing, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(billing.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, billing.DefaultRef())
}

// Cache resolves the wired cache service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Cache() (cache.Cache, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(cache.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, cache.DefaultRef())
}

// CDN resolves the wired cdn service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) CDN() (cdn.CDN, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(cdn.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, cdn.DefaultRef())
}

// Crypto resolves the wired crypto service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Crypto() (crypto.Crypto, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(crypto.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, crypto.DefaultRef())
}

// DB resolves the wired db service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) DB() (db.DB, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(db.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, db.DefaultRef())
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

// Document resolves the wired document service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Document() (document.Document, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(document.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, document.DefaultRef())
}

// EventBus resolves the wired eventbus service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) EventBus() (eventbus.EventBus, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(eventbus.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, eventbus.DefaultRef())
}

// Eventbus aliases EventBus for compatibility.
func (c *Container) Eventbus() (eventbus.EventBus, error) {
	return c.EventBus()
}

// Flag resolves the wired flag service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Flag() (flag.Flag, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(flag.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, flag.DefaultRef())
}

// Geo resolves the wired geo service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Geo() (geo.Geo, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(geo.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, geo.DefaultRef())
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
		return grpcclient.New(c.rootCtx, target, opts...)
	})
}

// I18n resolves the wired i18n service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) I18n() (i18n.I18n, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(i18n.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, i18n.DefaultRef())
}

// Idempotency resolves the wired idempotency service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Idempotency() (idempotency.Store, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(idempotency.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, idempotency.DefaultRef())
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

// Lock resolves the wired lock service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Lock() (lock.Locker, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(lock.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, lock.DefaultRef())
}

// Log resolves the wired log service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Log() (log.Logger, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(log.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, log.DefaultRef())
}

// Mailer resolves the wired mailer service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Mailer() (mailer.Mailer, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(mailer.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, mailer.DefaultRef())
}

// Media resolves the wired media service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Media() (media.Media, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(media.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, media.DefaultRef())
}

// Notification resolves the wired notification service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Notification() (notification.Notifier, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(notification.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, notification.DefaultRef())
}

// Observability resolves the wired observability service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Observability() (observability.Provider, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(observability.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, observability.DefaultRef())
}

// Outbox resolves the wired outbox service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Outbox() (outbox.Store, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(outbox.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, outbox.DefaultRef())
}

// Password resolves the wired password service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Password() (password.Hasher, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(password.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, password.DefaultRef())
}

// Payment resolves the wired payment service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Payment() (payment.Payment, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(payment.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, payment.DefaultRef())
}

// Permission resolves the wired permission service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Permission() (permission.Checker, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(permission.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, permission.DefaultRef())
}

// Queue resolves the wired queue service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Queue() (queue.Queue, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(queue.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, queue.DefaultRef())
}

// RAG builds a *rag.Engine over the resolved AI and VectorStore backends. It
// has no adapter registry and no config entry. Like Job, it shares the
// process-wide backend instances. Options apply only on first resolution;
// later calls return the cached singleton. WithHybridSearch additionally
// resolves the Search backend and builds via rag.NewHybrid.
func (c *Container) RAG(opts ...RAGOption) (*rag.Engine, error) {
	return c.rag.get(func() (*rag.Engine, error) {
		a, err := c.AI()
		if err != nil {
			return nil, fmt.Errorf("container: rag: resolve ai: %w", err)
		}

		vs, err := c.VectorStore()
		if err != nil {
			return nil, fmt.Errorf("container: rag: resolve vectorstore: %w", err)
		}

		cfg := ragConfig{options: rag.Options{Model: c.cfg.AI.Options.Model}}
		for _, opt := range opts {
			if opt == nil {
				continue
			}

			opt(&cfg)
		}

		if cfg.hybrid {
			s, searchErr := c.Search()
			if searchErr != nil {
				return nil, fmt.Errorf("container: rag: resolve search: %w", searchErr)
			}

			var engine *rag.Engine
			engine, err = rag.NewHybrid(a, vs, s, cfg.options)
			if err != nil {
				return nil, fmt.Errorf("container: rag: %w", err)
			}

			return engine, nil
		}

		engine, err := rag.New(a, vs, cfg.options)
		if err != nil {
			return nil, fmt.Errorf("container: rag: %w", err)
		}

		return engine, nil
	})
}

// RateLimit resolves the wired ratelimit service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) RateLimit() (ratelimit.Limiter, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(ratelimit.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, ratelimit.DefaultRef())
}

// Ratelimit aliases RateLimit for compatibility.
func (c *Container) Ratelimit() (ratelimit.Limiter, error) {
	return c.RateLimit()
}

// Resilience resolves the wired resilience service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Resilience() (resilience.Manager, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(resilience.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, resilience.DefaultRef())
}

// Router resolves the wired router service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Router() (router.Router, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(router.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, router.DefaultRef())
}

// Scheduler resolves the wired scheduler service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Scheduler() (scheduler.Scheduler, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(scheduler.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, scheduler.DefaultRef())
}

// Search resolves the wired search service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Search() (search.Search, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(search.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, search.DefaultRef())
}

// Secrets resolves the wired secrets service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Secrets() (secrets.Secrets, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(secrets.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, secrets.DefaultRef())
}

// Session resolves the wired session service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Session() (session.Store, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(session.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, session.DefaultRef())
}

// Storage resolves the wired storage service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Storage() (storage.Storage, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(storage.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, storage.DefaultRef())
}

// Tenant resolves the wired tenant service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Tenant() (tenant.Tenant, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(tenant.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, tenant.DefaultRef())
}

// VectorStore resolves the wired vectorstore service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) VectorStore() (vectorstore.VectorStore, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(vectorstore.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, vectorstore.DefaultRef())
}

// Webhook resolves the wired webhook service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Webhook() (webhook.Webhook, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(webhook.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, webhook.DefaultRef())
}

// Workflow resolves the wired workflow service instance. It fails with
// NotWiredError when New received no provider for the default ref.
func (c *Container) Workflow() (workflow.Workflow, error) {
	if c.provDefs != nil {
		if v, ok, err := c.providedDefault(workflow.DefaultRef()); ok {
			return v, err
		}
	}
	return Get(c, workflow.DefaultRef())
}
