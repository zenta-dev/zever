package container

import (
	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/analytics"
	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/billing"
	"github.com/zenta-dev/zever/cache"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/crypto"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/document"
	"github.com/zenta-dev/zever/eventbus"
	"github.com/zenta-dev/zever/flag"
	"github.com/zenta-dev/zever/geo"
	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/job"
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
