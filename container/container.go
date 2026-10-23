package container

import (
	"context"
	"sync"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/config"
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
)

// Container holds one lazily resolved instance per service.
// Nothing opens at construction; each accessor builds its service on first
// use and shares that instance (and its connections) process-wide.
type Container struct {
	cfg *config.Config

	ai            lazy[ai.AI]
	agent         lazy[*agent.Loop]
	analytics     lazy[analytics.Analytics]
	auth          lazy[auth.Auth]
	billing       lazy[billing.Billing]
	cache         lazy[cache.Cache]
	cdn           lazy[cdn.CDN]
	crypto        lazy[crypto.Crypto]
	db            lazy[db.DB]
	document      lazy[document.Document]
	eventbus      lazy[eventbus.EventBus]
	flag          lazy[flag.Flag]
	geo           lazy[geo.Geo]
	grpcServer    lazy[*grpc.Server]
	grpcClients   lazy[grpcClientCache]
	i18n          lazy[i18n.I18n]
	idempotency   lazy[idempotency.Store]
	job           lazy[*job.Dispatcher]
	lock          lazy[lock.Locker]
	log           lazy[log.Logger]
	mailer        lazy[mailer.Mailer]
	media         lazy[media.Media]
	notification  lazy[notification.Notifier]
	observability lazy[observability.Provider]
	outbox        lazy[outbox.Store]
	password      lazy[password.Hasher]
	payment       lazy[payment.Payment]
	permission    lazy[permission.Checker]
	queue         lazy[queue.Queue]
	rag           lazy[*rag.Engine]
	ratelimit     lazy[ratelimit.Limiter]
	resilience    lazy[resilience.Manager]
	router        lazy[router.Router]
	scheduler     lazy[scheduler.Scheduler]
	search        lazy[search.Search]
	secrets       lazy[secrets.Secrets]
	session       lazy[session.Store]
	storage       lazy[storage.Storage]
	tenant        lazy[tenant.Tenant]
	vectorstore   lazy[vectorstore.VectorStore]
	webhook       lazy[webhook.Webhook]
	workflow      lazy[workflow.Workflow]

	// rootCtx is the context for work a lazy build starts that outlives the
	// build call (e.g. the gRPC client connection). Builders run on whichever
	// goroutine first resolves a service, so they deliberately do not see that
	// caller's cancellation, deadline, or values: a canceled first request
	// must not poison a process-wide singleton. rootCtx is detached from any
	// caller (WithoutCancel over Background: no inherited cancellation, no
	// deadline, no values) but is canceled by Close so such work winds down
	// with the container.
	//nolint:containedctx // container-lifetime root: detached from callers by design, canceled in Close, never derived from request ctx
	rootCtx    context.Context
	cancelRoot context.CancelFunc

	// pools maps shareable DSNs to one shared db.DB pool per key (see
	// pools.go). Borrowers resolve through it on exact-DSN matches.
	pools *poolRegistry

	// provDefs holds one entry per Provide call, keyed by service/name. It
	// is written once by New and read-only after, so resolution reads it
	// without locking. Empty (nil) when New received no providers, in
	// which case every accessor fails with NotWiredError.
	provDefs map[refKey]*provider
	// provOrder is the declaration order of provDefs, the deterministic
	// tiebreak for eager builds, Wired, and Close.
	provOrder []refKey
	// provErrs records graph validation findings (duplicate Refs, missing
	// deps, cycles) per dependent Ref, plus failed eager builds. Get
	// surfaces them at resolution; validation itself opens nothing.
	provErrs map[refKey]error

	provMu sync.Mutex
	// provVals caches successful provider resolutions. A failed build is
	// evicted, so the next resolution retries (same contract as lazy.go).
	provVals map[refKey]*provEntry

	// env is the process environment, resolved once by resolveEnv on
	// first use. Every provider build seeds its Options from the
	// service defaults and overlays env before WithOptions appliers run.
	// envErr records a failed resolution and fails every provider build.
	env    config.Env
	envErr error
	// envOnce guards the one-shot environment resolution: the process
	// environment is read at most once per container, and only when a
	// provider build actually needs it.
	envOnce sync.Once

	pluginsMu sync.Mutex
	plugins   map[string]*pluginEntry
}

func (c *Container) sealed() {}

// New builds a Container from cfg and explicit wiring. Nothing
// opens yet except Eager providers: every other service resolves on first
// use through its Provider (see Provide). A nil cfg falls back to
// config.Default() for options-only settings (selectors, gates).
//
// The process environment resolves lazily, on the first provider build
// (see resolveEnv), and seeds every provided Ref: service defaults from
// DefaultOptions overlaid by ApplyEnv, then WithOptions appliers. An
// unreadable environment fails provider builds, and a container without
// providers never reads the process environment at all.
//
// The full provider graph is validated at construction (duplicate Refs,
// missing non-Optional dependencies, dependency cycles); findings are
// recorded per Ref and surfaced by Get and the accessors, so validation
// never opens anything. With zero providers every accessor fails with
// NotWiredError.
func New(cfg *config.Config, providers ...Provider) *Container {
	if cfg == nil {
		cfg = config.Default()
	}

	root, cancel := context.WithCancel(context.WithoutCancel(context.Background()))

	c := &Container{cfg: cfg, rootCtx: root, cancelRoot: cancel, pools: newPoolRegistry()}

	if len(providers) == 0 {
		return c
	}

	defs := make([]*provider, 0, len(providers))

	for _, p := range providers {
		if p == nil {
			continue
		}

		if def, ok := p.(*provider); ok {
			defs = append(defs, def)
			c.provOrder = append(c.provOrder, def.key)
		}
	}

	c.provDefs = make(map[refKey]*provider, len(defs))

	for _, def := range defs {
		if _, dup := c.provDefs[def.key]; !dup {
			c.provDefs[def.key] = def
		}
	}

	c.provErrs = validateProviders(defs)
	c.provVals = make(map[refKey]*provEntry)

	for _, key := range c.provOrder {
		def, ok := c.provDefs[key]
		if !ok || !def.eager {
			continue
		}

		if _, bad := c.provErrs[key]; bad {
			continue
		}

		if _, err := c.resolveProvider(key); err != nil {
			c.provErrs[key] = err
		}
	}

	return c
}
