package container

import (
	"sync"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/config"
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

// Container holds one lazily resolved instance per service.
// Nothing opens at construction; each accessor builds its service on first
// use and shares that instance (and its connections) process-wide.
type Container struct {
	cfg *config.Config

	ai            lazy[ai.AI]
	analytics     lazy[analytics.Analytics]
	auth          lazy[auth.Auth]
	billing       lazy[billing.Billing]
	cache         lazy[cache.Cache]
	crypto        lazy[crypto.Crypto]
	db            lazy[db.DB]
	document      lazy[document.Document]
	eventbus      lazy[eventbus.EventBus]
	flag          lazy[flag.Flag]
	geo           lazy[geo.Geo]
	grpcServer    lazy[*grpc.Server]
	i18n          lazy[i18n.I18n]
	idempotency   lazy[idempotency.Store]
	job           lazy[*job.Dispatcher]
	lock          lazy[lock.Locker]
	log           lazy[log.Logger]
	mailer        lazy[mailer.Mailer]
	media         lazy[media.Media]
	notification  lazy[notification.Notifier]
	observability lazy[observability.Provider]
	password      lazy[password.Hasher]
	payment       lazy[payment.Payment]
	permission    lazy[permission.Checker]
	queue         lazy[queue.Queue]
	ratelimit     lazy[ratelimit.Limiter]
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

	pluginsMu sync.Mutex
	plugins   map[string]*pluginEntry
}

func (c *Container) sealed() {}

// New builds a Container from cfg. Nothing is opened yet: every service
// resolves on first use. A nil cfg falls back to config.Default().
func New(cfg *config.Config) *Container {
	if cfg == nil {
		cfg = config.Default()
	}

	return &Container{cfg: cfg}
}
