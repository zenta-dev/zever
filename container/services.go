package container

import (
	"fmt"
	"sync"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/ai/anthropic"
	"github.com/zenta-dev/zever/ai/gemini"
	aiollama "github.com/zenta-dev/zever/ai/ollama"
	"github.com/zenta-dev/zever/ai/openai"
	"github.com/zenta-dev/zever/analytics"
	analyticslog "github.com/zenta-dev/zever/analytics/log"
	"github.com/zenta-dev/zever/analytics/posthog"
	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/auth/jwt"
	"github.com/zenta-dev/zever/auth/oidc"
	authsession "github.com/zenta-dev/zever/auth/session"
	"github.com/zenta-dev/zever/billing"
	billingpaddle "github.com/zenta-dev/zever/billing/paddle"
	billingstripe "github.com/zenta-dev/zever/billing/stripe"
	billingstub "github.com/zenta-dev/zever/billing/stub"
	"github.com/zenta-dev/zever/cache"
	cachememory "github.com/zenta-dev/zever/cache/memory"
	cacheredis "github.com/zenta-dev/zever/cache/redis"
	"github.com/zenta-dev/zever/crypto"
	cryptolocal "github.com/zenta-dev/zever/crypto/local"
	"github.com/zenta-dev/zever/db"
	dbpostgres "github.com/zenta-dev/zever/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/db/sqlite"
	"github.com/zenta-dev/zever/document"
	documentlatex "github.com/zenta-dev/zever/document/latex"
	documentlocal "github.com/zenta-dev/zever/document/local"
	documentremote "github.com/zenta-dev/zever/document/remote"
	"github.com/zenta-dev/zever/eventbus"
	eventbusmemory "github.com/zenta-dev/zever/eventbus/memory"
	eventbusredis "github.com/zenta-dev/zever/eventbus/redis"
	"github.com/zenta-dev/zever/flag"
	flagfirebase "github.com/zenta-dev/zever/flag/firebase"
	flagstatic "github.com/zenta-dev/zever/flag/static"
	"github.com/zenta-dev/zever/geo"
	geogoogle "github.com/zenta-dev/zever/geo/google"
	geoosm "github.com/zenta-dev/zever/geo/osm"
	geostatic "github.com/zenta-dev/zever/geo/static"
	"github.com/zenta-dev/zever/i18n"
	i18nembed "github.com/zenta-dev/zever/i18n/embed"
	i18nremote "github.com/zenta-dev/zever/i18n/remote"
	"github.com/zenta-dev/zever/idempotency"
	idempotencymemory "github.com/zenta-dev/zever/idempotency/memory"
	idempotencyredis "github.com/zenta-dev/zever/idempotency/redis"
	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/lock"
	lockmemory "github.com/zenta-dev/zever/lock/memory"
	lockredis "github.com/zenta-dev/zever/lock/redis"
	"github.com/zenta-dev/zever/log"
	lognoop "github.com/zenta-dev/zever/log/noop"
	logpretty "github.com/zenta-dev/zever/log/pretty"
	"github.com/zenta-dev/zever/log/slog"
	"github.com/zenta-dev/zever/log/zerolog"
	"github.com/zenta-dev/zever/mailer"
	mailerlog "github.com/zenta-dev/zever/mailer/log"
	"github.com/zenta-dev/zever/mailer/smtp"
	"github.com/zenta-dev/zever/media"
	medialocal "github.com/zenta-dev/zever/media/local"
	medias3 "github.com/zenta-dev/zever/media/s3"
	"github.com/zenta-dev/zever/notification"
	notificationfcm "github.com/zenta-dev/zever/notification/fcm"
	notificationlog "github.com/zenta-dev/zever/notification/log"
	notificationtwilio "github.com/zenta-dev/zever/notification/twilio"
	"github.com/zenta-dev/zever/observability"
	observabilitynoop "github.com/zenta-dev/zever/observability/noop"
	"github.com/zenta-dev/zever/observability/otlp"
	"github.com/zenta-dev/zever/observability/stdout"
	"github.com/zenta-dev/zever/password"
	"github.com/zenta-dev/zever/password/argon2"
	"github.com/zenta-dev/zever/payment"
	paymentpaddle "github.com/zenta-dev/zever/payment/paddle"
	paymentstripe "github.com/zenta-dev/zever/payment/stripe"
	paymentstub "github.com/zenta-dev/zever/payment/stub"
	"github.com/zenta-dev/zever/permission"
	permissioncasbin "github.com/zenta-dev/zever/permission/casbin"
	permissionnoop "github.com/zenta-dev/zever/permission/noop"
	permissionrbac "github.com/zenta-dev/zever/permission/rbac"
	"github.com/zenta-dev/zever/queue"
	queuememory "github.com/zenta-dev/zever/queue/memory"
	queueredis "github.com/zenta-dev/zever/queue/redis"
	"github.com/zenta-dev/zever/ratelimit"
	ratelimitmemory "github.com/zenta-dev/zever/ratelimit/memory"
	ratelimitredis "github.com/zenta-dev/zever/ratelimit/redis"
	"github.com/zenta-dev/zever/router"
	routerfiber "github.com/zenta-dev/zever/router/fiber"
	routerstdhttp "github.com/zenta-dev/zever/router/stdhttp"
	"github.com/zenta-dev/zever/scheduler"
	schedulerembedded "github.com/zenta-dev/zever/scheduler/embedded"
	"github.com/zenta-dev/zever/search"
	searchmeilisearch "github.com/zenta-dev/zever/search/meilisearch"
	searchpostgres "github.com/zenta-dev/zever/search/postgres"
	searchsqlite "github.com/zenta-dev/zever/search/sqlite"
	"github.com/zenta-dev/zever/secrets"
	secretsenv "github.com/zenta-dev/zever/secrets/env"
	"github.com/zenta-dev/zever/session"
	sessionmemory "github.com/zenta-dev/zever/session/memory"
	sessionredis "github.com/zenta-dev/zever/session/redis"
	"github.com/zenta-dev/zever/storage"
	storagelocal "github.com/zenta-dev/zever/storage/local"
	storager2 "github.com/zenta-dev/zever/storage/r2"
	storages3 "github.com/zenta-dev/zever/storage/s3"
	"github.com/zenta-dev/zever/tenant"
	tenantheader "github.com/zenta-dev/zever/tenant/header"
	tenantsingle "github.com/zenta-dev/zever/tenant/single"
	"github.com/zenta-dev/zever/vectorstore"
	vectorstorepgvector "github.com/zenta-dev/zever/vectorstore/pgvector"
	vectorstoreqdrant "github.com/zenta-dev/zever/vectorstore/qdrant"
	vectorstoresqlite "github.com/zenta-dev/zever/vectorstore/sqlite"
	"github.com/zenta-dev/zever/webhook"
	webhookhttp "github.com/zenta-dev/zever/webhook/http"
	webhookqueue "github.com/zenta-dev/zever/webhook/queue"
	webhooksqlite "github.com/zenta-dev/zever/webhook/sqlite"
	"github.com/zenta-dev/zever/workflow"
	workflowmemory "github.com/zenta-dev/zever/workflow/memory"
)

// adaptersOnce guards the process-wide adapter registration below.
var adaptersOnce sync.Once

// ensureAdapters registers every service adapter factory exactly once per
// process. Adapter packages expose constructors but never self-register,
// so the container — the composition root — wires them before first use.
// Registration only fills factory maps; it opens nothing and starts no
// background work. Duplicate errors are ignored: every factory passed here
// is valid by construction, so the only failure mode is a factory the host
// application already registered, which is benign.
func ensureAdapters() {
	adaptersOnce.Do(registerAdapters)
}

func registerAdapters() {
	_ = ai.Register(ai.Anthropic, anthropic.New)
	_ = ai.Register(ai.OpenAI, openai.New)
	_ = ai.Register(ai.Gemini, gemini.New)
	_ = ai.Register(ai.Ollama, aiollama.New)

	_ = analytics.Register(analytics.Log, analyticslog.New)
	_ = analytics.Register(analytics.PostHog, posthog.New)

	_ = auth.Register(auth.JWT, jwt.New)
	_ = auth.Register(auth.Session, authsession.New)
	_ = auth.Register(auth.OIDC, oidc.New)

	_ = billing.Register(billing.Stub, billingstub.Open)
	_ = billing.Register(billing.Stripe, billingstripe.New)
	_ = billing.Register(billing.Paddle, billingpaddle.New)

	_ = cache.Register(cache.Memory, cachememory.New)
	_ = cache.Register(cache.Redis, cacheredis.New)

	_ = crypto.Register(crypto.AdapterLocal, cryptolocal.New)

	_ = db.Register(db.SQLite, dbsqlite.New)
	_ = db.Register(db.Postgres, dbpostgres.New)

	_ = document.Register(document.Local, documentlocal.New)
	_ = document.Register(document.Remote, documentremote.New)
	_ = document.Register(document.Latex, documentlatex.New)

	_ = eventbus.Register(eventbus.Memory, eventbusmemory.New)
	_ = eventbus.Register(eventbus.Redis, eventbusredis.New)

	_ = flag.Register(flag.Static, flagstatic.New)
	_ = flag.Register(flag.Firebase, flagfirebase.New)

	_ = geo.Register(geo.Google, geogoogle.New)
	_ = geo.Register(geo.Static, geostatic.New)
	_ = geo.Register(geo.OSM, geoosm.New)

	_ = i18n.Register(i18n.Embed, i18nembed.New)
	_ = i18n.Register(i18n.Remote, i18nremote.New)

	_ = idempotency.Register(idempotency.Memory, idempotencymemory.New)
	_ = idempotency.Register(idempotency.Redis, idempotencyredis.New)

	_ = lock.Register(lock.Memory, lockmemory.New)
	_ = lock.Register(lock.Redis, lockredis.New)

	_ = log.Register(log.Noop, func(log.Options) (log.Logger, error) { return lognoop.New(), nil })
	_ = log.Register(log.ZeroLog, func(o log.Options) (log.Logger, error) { return zerolog.New(o), nil })
	_ = log.Register(log.Slog, func(o log.Options) (log.Logger, error) { return slog.New(o), nil })
	_ = log.Register(log.Pretty, func(o log.Options) (log.Logger, error) { return logpretty.New(o), nil })

	_ = mailer.Register(mailer.Log, mailerlog.New)
	_ = mailer.Register(mailer.SMTP, smtp.New)

	_ = media.Register(media.Local, medialocal.New)
	_ = media.Register(media.S3, medias3.New)

	_ = notification.Register(notification.Log, notificationlog.New)
	_ = notification.Register(notification.Twilio, notificationtwilio.New)
	_ = notification.Register(notification.FCM, notificationfcm.New)

	_ = observability.Register(observability.Noop, func(observability.Options) (observability.Provider, error) {
		return observabilitynoop.New(), nil
	})
	_ = observability.Register(observability.Stdout, stdout.New)
	_ = observability.Register(observability.OTLP, otlp.New)

	_ = password.Register(password.AdapterArgon2ID, argon2.New)

	_ = payment.Register(payment.Stub, paymentstub.New)
	_ = payment.Register(payment.Stripe, paymentstripe.New)
	_ = payment.Register(payment.Paddle, paymentpaddle.New)

	_ = permission.Register(permission.Noop, permissionnoop.New)
	_ = permission.Register(permission.RBAC, permissionrbac.New)
	_ = permission.Register(permission.Casbin, permissioncasbin.New)

	_ = queue.Register(queue.Memory, queuememory.New)
	_ = queue.Register(queue.Redis, queueredis.New)

	_ = ratelimit.Register(ratelimit.Memory, ratelimitmemory.New)
	_ = ratelimit.Register(ratelimit.Redis, ratelimitredis.New)

	_ = router.Register(router.AdapterFiber, routerfiber.New)
	_ = router.Register(router.AdapterStdHTTP, routerstdhttp.New)

	_ = scheduler.Register(scheduler.Embedded, schedulerembedded.New)

	_ = search.Register(search.Postgres, searchpostgres.New)
	_ = search.Register(search.Meilisearch, searchmeilisearch.New)
	_ = search.Register(search.SQLite, searchsqlite.New)

	_ = secrets.Register(secrets.Env, func(o secrets.Options) (secrets.Secrets, error) {
		return secretsenv.New(secretsenv.Options{Prefix: o.Prefix})
	})

	_ = session.Register(session.Memory, sessionmemory.New)
	_ = session.Register(session.Redis, sessionredis.New)

	_ = storage.Register(storage.AdapterLocal, storagelocal.New)
	_ = storage.Register(storage.AdapterS3, storages3.New)
	_ = storage.Register(storage.AdapterR2, storager2.New)

	_ = tenant.Register(tenant.Single, tenantsingle.New)
	_ = tenant.Register(tenant.Header, tenantheader.New)

	_ = vectorstore.Register(vectorstore.SQLite, vectorstoresqlite.New)
	_ = vectorstore.Register(vectorstore.PGVector, vectorstorepgvector.New)
	_ = vectorstore.Register(vectorstore.Qdrant, vectorstoreqdrant.New)

	_ = webhook.Register(webhook.AdapterHTTP, webhookhttp.New)
	_ = webhook.Register(webhook.AdapterQueue, webhookqueue.New)
	_ = webhook.Register(webhook.AdapterSQLite, webhooksqlite.New)

	_ = workflow.Register(workflow.Memory, workflowmemory.New)
}

// openService parses a service's adapter name, then opens it with typed
// opts. The adapter string never reaches Open: parse failures short-circuit
// before any construction. Wrapping adds only the service name, so
// secret-bearing option values never leak into error text.
func openService[T any, A any, O any](service string, name string, parse func(string) (A, error), open func(A, O) (T, error), opts O) (T, error) {
	ensureAdapters()

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
